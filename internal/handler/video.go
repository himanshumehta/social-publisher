package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/middleware"
)

func (h *Handler) ListVideos(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	videos, total, err := h.services.Video.ListByProject(r.Context(), id, limit, offset)
	if err != nil {
		h.logger.Error("list videos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list videos")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  videos,
		"total": total,
	})
}

func (h *Handler) GetVideoPlaybackURL(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "videoId", "video id")
	if !ok {
		return
	}

	url, err := h.services.Video.GetPlaybackURL(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"playback_url": url})
}

func (h *Handler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	id, ok := h.requirePathUUID(w, r, "videoId", "video id")
	if !ok {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	if err := h.services.Video.SoftDelete(r.Context(), id, userID, role); err != nil {
		h.logger.Error("delete video failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "video deleted"})
}
