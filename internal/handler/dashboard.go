package handler

import (
	"net/http"
	"strconv"
)

func (h *Handler) GetCreatorDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	stats, err := h.services.Dashboard.GetCreatorStats(r.Context(), userID)
	if err != nil {
		h.logger.Error("get creator dashboard stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get dashboard stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetAdminDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.services.Dashboard.GetAdminStats(r.Context())
	if err != nil {
		h.logger.Error("get admin dashboard stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get admin stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) ListProjectPerformance(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	data, total, err := h.services.Dashboard.ListProjectPerformance(r.Context(), &userID, limit, offset)
	if err != nil {
		h.logger.Error("list project performance failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list project performance")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  data,
		"total": total,
	})
}

func (h *Handler) ListAllProjectPerformance(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	data, total, err := h.services.Dashboard.ListProjectPerformance(r.Context(), nil, limit, offset)
	if err != nil {
		h.logger.Error("list all project performance failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list project performance")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  data,
		"total": total,
	})
}

func (h *Handler) GetVideoPerformance(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	data, err := h.services.Dashboard.GetVideoPerformance(r.Context(), id)
	if err != nil {
		h.logger.Error("get video performance failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get video performance")
		return
	}

	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) GetPlatformStats(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "videoId", "video id")
	if !ok {
		return
	}

	data, err := h.services.Dashboard.GetPlatformStats(r.Context(), id)
	if err != nil {
		h.logger.Error("get platform stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get platform stats")
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// --- Flow 9: Enhanced Dashboard Handlers ---

func (h *Handler) GetEarningsTrend(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	if months <= 0 || months > 12 {
		months = 6
	}
	data, err := h.services.Dashboard.GetEarningsTrend(r.Context(), userID, months)
	if err != nil {
		h.logger.Error("get earnings trend failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get earnings trend")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) GetTopVideos(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	data, err := h.services.Dashboard.GetTopVideos(r.Context(), userID, period, limit)
	if err != nil {
		h.logger.Error("get top videos failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get top videos")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) GetPlatformBreakdown(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	data, err := h.services.Dashboard.GetPlatformBreakdown(r.Context(), userID, period)
	if err != nil {
		h.logger.Error("get platform breakdown failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get platform breakdown")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) GetActivityFeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	data, err := h.services.Dashboard.GetActivityFeed(r.Context(), userID, limit)
	if err != nil {
		h.logger.Error("get activity feed failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get activity feed")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) GetCurrentMonthPayout(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	data, err := h.services.Dashboard.GetCurrentMonthPayout(r.Context(), userID)
	if err != nil {
		h.logger.Error("get current month payout failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get current month payout")
		return
	}
	writeJSON(w, http.StatusOK, data)
}
