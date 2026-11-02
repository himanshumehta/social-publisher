package handler

import (
	"net/http"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// GetCalendar handles GET /api/v1/calendar?start=2024-01-01&end=2024-01-31
func (h *Handler) GetCalendar(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		writeError(w, http.StatusBadRequest, "start and end query parameters are required (YYYY-MM-DD)")
		return
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start date format, use YYYY-MM-DD")
		return
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end date format, use YYYY-MM-DD")
		return
	}

	// Make end inclusive: extend to end of day
	end = end.Add(24 * time.Hour)

	if end.Before(start) {
		writeError(w, http.StatusBadRequest, "end date must be after start date")
		return
	}

	entries, err := h.services.Calendar.GetCalendar(r.Context(), userID, start, end)
	if err != nil {
		h.logger.Error("get calendar failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get calendar")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": safeSlice(entries),
	})
}

// SchedulePost handles PATCH /api/v1/posts/{id}/schedule
func (h *Handler) SchedulePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	var req model.SchedulePostRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if req.ScheduledAt.IsZero() {
		writeError(w, http.StatusBadRequest, "scheduled_at is required")
		return
	}

	err := h.services.Calendar.SchedulePost(r.Context(), userID, postID, req.ScheduledAt, req.Timezone)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "scheduled"})
}

// BulkSchedulePost handles POST /api/v1/posts/bulk-schedule
func (h *Handler) BulkSchedulePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.BulkScheduleRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "at least one item is required")
		return
	}

	if len(req.Items) > 50 {
		writeError(w, http.StatusBadRequest, "maximum 50 items per bulk schedule request")
		return
	}

	results, err := h.services.Calendar.BulkSchedule(r.Context(), userID, req.Items)
	if err != nil {
		h.logger.Error("bulk schedule failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to bulk schedule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

// UnschedulePost handles DELETE /api/v1/posts/{id}/schedule
func (h *Handler) UnschedulePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	err := h.services.Calendar.UnschedulePost(r.Context(), userID, postID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unscheduled"})
}
