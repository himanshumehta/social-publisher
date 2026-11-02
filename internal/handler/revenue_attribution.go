package handler

import (
	"net/http"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// Revenue Sources
// ---------------------------------------------------------------------------

// AddRevenueSource handles POST /api/v1/revenue/sources
func (h *Handler) AddRevenueSource(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateRevenueSourceRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	source, err := h.services.RevenueAttribution.AddSource(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, source)
}

// ListRevenueSources handles GET /api/v1/revenue/sources?start=...&end=...
func (h *Handler) ListRevenueSources(w http.ResponseWriter, r *http.Request) {
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

	sources, err := h.services.RevenueAttribution.ListSources(r.Context(), userID, start, end)
	if err != nil {
		h.logger.Error("list revenue sources failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list revenue sources")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sources": safeSlice(sources),
	})
}

// UpdateRevenueSource handles PUT /api/v1/revenue/sources/{id}
func (h *Handler) UpdateRevenueSource(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	sourceID, ok := h.requirePathUUID(w, r, "id", "source ID")
	if !ok {
		return
	}

	var req model.CreateRevenueSourceRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	source, err := h.services.RevenueAttribution.UpdateSource(r.Context(), userID, sourceID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, source)
}

// DeleteRevenueSource handles DELETE /api/v1/revenue/sources/{id}
func (h *Handler) DeleteRevenueSource(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	sourceID, ok := h.requirePathUUID(w, r, "id", "source ID")
	if !ok {
		return
	}

	err := h.services.RevenueAttribution.DeleteSource(r.Context(), userID, sourceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Revenue Breakdown
// ---------------------------------------------------------------------------

// GetRevenueBreakdown handles GET /api/v1/revenue/breakdown?start=...&end=...
func (h *Handler) GetRevenueBreakdown(w http.ResponseWriter, r *http.Request) {
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

	breakdown, err := h.services.RevenueAttribution.GetBreakdown(r.Context(), userID, start, end)
	if err != nil {
		h.logger.Error("get revenue breakdown failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get revenue breakdown")
		return
	}

	writeJSON(w, http.StatusOK, breakdown)
}

// ---------------------------------------------------------------------------
// Revenue Goals
// ---------------------------------------------------------------------------

// CreateRevenueGoal handles POST /api/v1/revenue/goals
func (h *Handler) CreateRevenueGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateGoalRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	goal, err := h.services.RevenueAttribution.CreateGoal(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, goal)
}

// ListRevenueGoals handles GET /api/v1/revenue/goals
func (h *Handler) ListRevenueGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	goals, err := h.services.RevenueAttribution.ListGoals(r.Context(), userID)
	if err != nil {
		h.logger.Error("list revenue goals failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list revenue goals")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"goals": safeSlice(goals),
	})
}

// GetGoalProgress handles GET /api/v1/revenue/goals/{id}/progress
func (h *Handler) GetGoalProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	goalID, ok := h.requirePathUUID(w, r, "id", "goal ID")
	if !ok {
		return
	}

	progress, err := h.services.RevenueAttribution.GetGoalProgress(r.Context(), userID, goalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, progress)
}

// ---------------------------------------------------------------------------
// Affiliate Links
// ---------------------------------------------------------------------------

// CreateAffiliateLink handles POST /api/v1/revenue/affiliate-links
func (h *Handler) CreateAffiliateLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateAffiliateLinkRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	link, err := h.services.RevenueAttribution.CreateAffiliateLink(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

// ListAffiliateLinks handles GET /api/v1/revenue/affiliate-links
func (h *Handler) ListAffiliateLinks(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	links, err := h.services.RevenueAttribution.ListAffiliateLinks(r.Context(), userID)
	if err != nil {
		h.logger.Error("list affiliate links failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list affiliate links")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"links": safeSlice(links),
	})
}

// TrackAffiliateClick handles GET /api/v1/r/{code}
// This is a PUBLIC endpoint (no auth). It increments the click count and
// redirects the user to the original URL.
func (h *Handler) TrackAffiliateClick(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "tracking code is required")
		return
	}

	originalURL, err := h.services.RevenueAttribution.TrackClick(r.Context(), code)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, originalURL, http.StatusFound)
}
