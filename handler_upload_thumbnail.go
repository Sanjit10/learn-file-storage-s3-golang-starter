package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

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

	media_type := header.Header.Get("Content-Type")
	image_data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read thumbnail file", err)
		return
	}

	video_metadata , err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video draft record", err)
		return
	}
	if userID != video_metadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized Request", errors.New("unauthorized request"))
		return
	}

	encoded_image := base64.StdEncoding.EncodeToString(image_data)
	videoThumbnail_dataUrl := fmt.Sprintf("data:%v;base64,%v", media_type, encoded_image)

	video_metadata.ThumbnailURL = &videoThumbnail_dataUrl

	db_err := cfg.db.UpdateVideo(video_metadata)
	if db_err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata", db_err)
		return
	}

	respondWithJSON(w, http.StatusOK, video_metadata)
}
