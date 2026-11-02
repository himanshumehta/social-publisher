package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// RegisterPushToken -- POST /api/v1/mobile/push-token
func (h *Handler) RegisterPushToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.RegisterTokenRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Mobile.RegisterToken(r.Context(), userID, &req); err != nil {
		h.logger.Error("register push token failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// UnregisterPushToken -- DELETE /api/v1/mobile/push-token
func (h *Handler) UnregisterPushToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device_id query parameter is required")
		return
	}

	if err := h.services.Mobile.UnregisterToken(r.Context(), userID, deviceID); err != nil {
		h.logger.Error("unregister push token failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

// GetSyncEntries -- GET /api/v1/mobile/sync?last_sequence=0&limit=50
func (h *Handler) GetSyncEntries(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	lastSequence, _ := strconv.ParseInt(q.Get("last_sequence"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	resp, err := h.services.Mobile.GetSyncEntries(r.Context(), userID, lastSequence, limit)
	if err != nil {
		h.logger.Error("get sync entries failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sync entries")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetMobileNotifications -- GET /api/v1/mobile/notifications?page=1&page_size=20
func (h *Handler) GetMobileNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	items, total, err := h.services.Mobile.GetNotificationFeed(r.Context(), userID, page, pageSize)
	if err != nil {
		h.logger.Error("get mobile notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get notifications")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  safeSlice(items),
		"total": total,
	})
}

// QuickCreatePost -- POST /api/v1/mobile/quick-post
func (h *Handler) QuickCreatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.QuickPostRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	postID, err := h.services.Mobile.QuickCreatePost(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("quick create post failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     postID,
		"status": "draft",
	})
}

// GetMobileDashboard -- GET /api/v1/mobile/dashboard
func (h *Handler) GetMobileDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	dash, err := h.services.Mobile.GetMobileDashboard(r.Context(), userID)
	if err != nil {
		h.logger.Error("get mobile dashboard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}

	writeJSON(w, http.StatusOK, dash)
}
