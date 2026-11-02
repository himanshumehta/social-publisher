package handler

import (
	"net/http"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// GetAnalyticsOverview
// ---------------------------------------------------------------------------

// GetAnalyticsOverview handles GET /api/v1/analytics/overview?start=...&end=...
func (h *Handler) GetAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	dr, err := parseDateRange(r, "start", "end")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date range: "+err.Error())
		return
	}

	overview, err := h.services.Analytics.GetOverview(r.Context(), userID, dr)
	if err != nil {
		h.logger.Error("get analytics overview", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get analytics overview")
		return
	}

	writeJSON(w, http.StatusOK, overview)
}

// ---------------------------------------------------------------------------
// GetPlatformAnalytics
// ---------------------------------------------------------------------------

// GetPlatformAnalytics handles GET /api/v1/analytics/platform/{platform}?start=...&end=...
func (h *Handler) GetPlatformAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	platform := r.PathValue("platform")
	if platform == "" {
		writeError(w, http.StatusBadRequest, "platform is required")
		return
	}

	dr, err := parseDateRange(r, "start", "end")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date range: "+err.Error())
		return
	}

	snapshots, err := h.services.Analytics.GetPlatformStats(r.Context(), userID, platform, dr)
	if err != nil {
		h.logger.Error("get platform analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get platform analytics")
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(snapshots))
}

// ---------------------------------------------------------------------------
// GetPostAnalyticsHandler
// ---------------------------------------------------------------------------

// GetPostAnalyticsHandler handles GET /api/v1/analytics/post/{id}
func (h *Handler) GetPostAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	analytics, err := h.services.Analytics.GetPostAnalytics(r.Context(), postID)
	if err != nil {
		h.logger.Error("get post analytics", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get post analytics")
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(analytics))
}

// ---------------------------------------------------------------------------
// GetAnalyticsComparison
// ---------------------------------------------------------------------------

// GetAnalyticsComparison handles
// GET /api/v1/analytics/comparison?current_start=...&current_end=...&previous_start=...&previous_end=...
func (h *Handler) GetAnalyticsComparison(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	current, err := parseDateRange(r, "current_start", "current_end")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid current date range: "+err.Error())
		return
	}

	previous, err := parseDateRange(r, "previous_start", "previous_end")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid previous date range: "+err.Error())
		return
	}

	comparison, err := h.services.Analytics.GetComparison(r.Context(), userID, current, previous)
	if err != nil {
		h.logger.Error("get analytics comparison", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get analytics comparison")
		return
	}

	writeJSON(w, http.StatusOK, comparison)
}

// ---------------------------------------------------------------------------
// parseDateRange (handler-level helper)
// ---------------------------------------------------------------------------

// parseDateRange extracts two query-string dates and returns a DateRange.
func parseDateRange(r *http.Request, startKey, endKey string) (model.DateRange, error) {
	const layout = "2006-01-02"

	startStr := r.URL.Query().Get(startKey)
	endStr := r.URL.Query().Get(endKey)

	start, err := time.Parse(layout, startStr)
	if err != nil {
		return model.DateRange{}, err
	}
	end, err := time.Parse(layout, endStr)
	if err != nil {
		return model.DateRange{}, err
	}

	return model.DateRange{Start: start, End: end}, nil
}
