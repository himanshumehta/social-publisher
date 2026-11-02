package handler

import (
	"context"
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) InitiateUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.InitiateUploadRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	resp, err := h.services.Upload.InitiateUpload(r.Context(), userID, req)
	if err != nil {
		if service.IsDuplicateFileError(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.logger.Error("initiate upload failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.CompleteUploadRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	video, err := h.services.Upload.CompleteUpload(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("complete upload failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Trigger thumbnail generation asynchronously
	// Use context.WithoutCancel so the goroutine isn't cancelled when the HTTP response is sent.
	go func() {
		bgCtx := context.WithoutCancel(r.Context())
		if err := h.services.Thumbnail.GenerateThumbnail(bgCtx, video.ID); err != nil {
			h.logger.Error("thumbnail generation failed", "video_id", video.ID, "error", err)
		} else {
			h.logger.Info("thumbnail generated successfully", "video_id", video.ID)
		}
	}()

	writeJSON(w, http.StatusOK, video)
}

func (h *Handler) AbortUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		UploadID string `json:"upload_id"`
	}
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Upload.AbortUpload(r.Context(), userID, req.UploadID); err != nil {
		h.logger.Error("abort upload failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "upload aborted"})
}

func (h *Handler) RefreshUploadURLs(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.RefreshURLsRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	resp, err := h.services.Upload.GetPresignedURLsForParts(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("refresh urls failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
