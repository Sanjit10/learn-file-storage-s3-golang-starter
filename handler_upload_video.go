package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)


func processVideoForFastStart(filePath string) (string, error) {
	outputPath := fmt.Sprintf("%v.processing.mp4", filePath)

	// Use ffmpeg, not ffprobe
	cmd := exec.Command(
		"ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "faststart",
		outputPath,
	)

	// Capture stderr (ffmpeg writes logs/errors there)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run command
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %v, details: %s", err, stderr.String())
	}

	return outputPath, nil
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	upload_limit := 1 << 30

	r.Body = http.MaxBytesReader(w, r.Body, int64(upload_limit))

	videoIDstr := r.PathValue("videoID")
	videoIDuuid, err := uuid.Parse(videoIDstr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}

	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "", err)
		return
	}

	userID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoIDuuid)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "", err)
		return
	}

	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "", err)
		return
	}

	videoData, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}
	defer videoData.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "", errors.New("wrong media type"))
		return
	}

	tempVideoFile, err := os.CreateTemp("./temp/", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}
	defer os.Remove(tempVideoFile.Name())
	defer tempVideoFile.Close()

	if _, err := io.Copy(tempVideoFile, videoData); err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to save upload", err)
		return
	}
	tempVideoFile.Seek(0, io.SeekStart)

	processed_file_path, err := processVideoForFastStart(tempVideoFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}

	processedFile, err := os.Open(processed_file_path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed file", err)
		return
	}
	defer os.Remove(processed_file_path)
	defer processedFile.Close()

	video_aspect_ratio , err := getVideoAspectRatio(processed_file_path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}

	var orientation string

	switch video_aspect_ratio {
		case "16:9":
			orientation = "landscape"
		case "9:16":
			orientation = "portrait"
		default:
			orientation = "other"
	}


	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random bytes", err)
		return
	}
	randomBase64 := base64.RawURLEncoding.EncodeToString(randomBytes)
	file_key := fmt.Sprintf("%v/%v.%v",orientation , randomBase64, "mp4")

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
    Bucket:      &cfg.s3Bucket,
    Key:         &file_key,
    Body:        processedFile,
    ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "S3 upload failed", err)
		return
	}


	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, file_key)
	videoMetadata.VideoURL = &videoURL

	if err := cfg.db.UpdateVideo(videoMetadata); err != nil {
		respondWithError(w, http.StatusBadRequest, "", err)
		return
	}

}
