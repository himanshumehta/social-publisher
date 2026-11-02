package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ConnectCommerce creates a new platform commerce integration.
// POST /api/v1/commerce/integrations
func (h *Handler) ConnectCommerce(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.ConnectCommerceRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	ci, err := h.services.Commerce.ConnectIntegration(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("connect commerce integration failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, ci)
}

// DisconnectCommerce disconnects a platform commerce integration.
// POST /api/v1/commerce/integrations/{id}/disconnect
func (h *Handler) DisconnectCommerce(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	integrationID, ok := h.requirePathUUID(w, r, "id", "integration id")
	if !ok {
		return
	}

	if err := h.services.Commerce.DisconnectIntegration(r.Context(), userID, integrationID); err != nil {
		h.logger.Error("disconnect commerce integration failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// ListCommerceIntegrations returns all commerce integrations for the user.
// GET /api/v1/commerce/integrations
func (h *Handler) ListCommerceIntegrations(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	list, err := h.services.Commerce.ListIntegrations(r.Context(), userID)
	if err != nil {
		h.logger.Error("list commerce integrations failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list integrations")
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(list))
}

// GetCommerceStats returns aggregated commerce statistics for a time period.
// GET /api/v1/commerce/stats?start=2024-01-01&end=2024-12-31
func (h *Handler) GetCommerceStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		// Default to last 30 days
		start = time.Now().AddDate(0, 0, -30)
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		end = time.Now()
	}

	stats, err := h.services.Commerce.GetStats(r.Context(), userID, start, end)
	if err != nil {
		h.logger.Error("get commerce stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// ListCommerceTransactions returns a paginated, filtered list of transactions.
// GET /api/v1/commerce/transactions?platform=youtube&type=super_chat&page=1
func (h *Handler) ListCommerceTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	filter := &model.TransactionFilter{
		Platform:        q.Get("platform"),
		TransactionType: q.Get("type"),
		Page:            page,
		PageSize:        pageSize,
	}

	if s := q.Get("start"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			filter.StartDate = t
		}
	}
	if s := q.Get("end"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			filter.EndDate = t
		}
	}

	txns, total, err := h.services.Commerce.ListTransactions(r.Context(), userID, filter)
	if err != nil {
		h.logger.Error("list commerce transactions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"transactions": safeSlice(txns),
		"total":        total,
		"page":         filter.Page,
		"page_size":    filter.PageSize,
	})
}

// SyncCommerceIntegration triggers an immediate sync for one integration.
// POST /api/v1/commerce/integrations/{id}/sync
func (h *Handler) SyncCommerceIntegration(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	integrationID, ok := h.requirePathUUID(w, r, "id", "integration id")
	if !ok {
		return
	}

	if err := h.services.Commerce.SyncIntegration(r.Context(), integrationID); err != nil {
		h.logger.Error("sync commerce integration failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}
