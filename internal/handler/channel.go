package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// ListChannels — GET /api/v1/admin/channels
func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
	var categoryID *uuid.UUID
	if cidStr := r.URL.Query().Get("category_id"); cidStr != "" {
		cid, err := uuid.Parse(cidStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid category_id")
			return
		}
		categoryID = &cid
	}

	platform := r.URL.Query().Get("platform")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	channels, total, err := h.services.Channel.List(r.Context(), categoryID, platform, limit, offset)
	if err != nil {
		h.logger.Error("list channels failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list channels")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  channels,
		"total": total,
	})
}

// CreateChannel — POST /api/v1/admin/channels
func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	var req service.CreateChannelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	channel, err := h.services.Channel.Create(r.Context(), req)
	if err != nil {
		h.logger.Error("create channel failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, channel)
}

// UpdateChannel — PUT /api/v1/admin/channels/{id}
func (h *Handler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	var req service.UpdateChannelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	channel, err := h.services.Channel.Update(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update channel failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, channel)
}

// DeleteChannel — DELETE /api/v1/admin/channels/{id}
func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	if err := h.services.Channel.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete channel failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
