package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"crypto/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get thumbnail", err)
		return
	}
	defer file.Close()

	media_type, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}
	media_type_slice := strings.Split(media_type, "/")
	if len(media_type_slice) != 2 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type format", errors.New("invalid media type format"))
		return
	}
	media_type_extension := media_type_slice[1]
	if media_type_extension != "png" && media_type_extension != "jpeg" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type format", err)
		return
	}
	video_metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video draft record", err)
		return
	}
	if userID != video_metadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized Request", errors.New("unauthorized request"))
		return
	}
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random bytes", err)
		return
	}
	randomBase64 := base64.RawURLEncoding.EncodeToString(randomBytes)

	file_path := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%v.%v", randomBase64, media_type_extension))
	new_file, err := os.Create(file_path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
		return
	}
	defer new_file.Close()
	_, copyErr := io.Copy(new_file, file)
	if copyErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save thumbnail", copyErr)
		return
	}

	video_metadata.ThumbnailURL = stringPtr(fmt.Sprintf("http://localhost:8091/assets/%v.%v", randomBase64, media_type_extension))

	db_err := cfg.db.UpdateVideo(video_metadata)
	if db_err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata", db_err)
		return
	}

	respondWithJSON(w, http.StatusOK, video_metadata)
}

func stringPtr(s string) *string {
	return &s
}
