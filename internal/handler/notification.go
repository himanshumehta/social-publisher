package handler

import (
	"net/http"
	"strconv"
)

// ListNotifications -- GET /api/v1/notifications
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	unreadOnly := q.Get("unread") == "true"
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	notifications, total, err := h.services.Notification.ListByUser(r.Context(), userID, unreadOnly, limit, offset)
	if err != nil {
		h.logger.Error("list notifications failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  notifications,
		"total": total,
	})
}

// GetUnreadCount -- GET /api/v1/notifications/unread-count
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	count, err := h.services.Notification.UnreadCount(r.Context(), userID)
	if err != nil {
		h.logger.Error("get unread count failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get unread count")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// MarkNotificationRead -- POST /api/v1/notifications/{id}/read
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	notifID, ok := h.requirePathUUID(w, r, "id", "notification id")
	if !ok {
		return
	}

	if err := h.services.Notification.MarkAsRead(r.Context(), userID, notifID); err != nil {
		h.logger.Error("mark notification read failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

// MarkAllNotificationsRead -- POST /api/v1/notifications/read-all
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	updated, err := h.services.Notification.MarkAllAsRead(r.Context(), userID)
	if err != nil {
		h.logger.Error("mark all notifications read failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to mark all as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"updated": updated})
}

// DeleteNotification -- DELETE /api/v1/notifications/{id}
func (h *Handler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	notifID, ok := h.requirePathUUID(w, r, "id", "notification id")
	if !ok {
		return
	}

	if err := h.services.Notification.Delete(r.Context(), userID, notifID); err != nil {
		h.logger.Error("delete notification failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
