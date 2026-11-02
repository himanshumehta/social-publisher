package handler

import (
	"net/http"
	"strconv"
)

func (h *Handler) GetMyPayouts(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	data, err := h.services.Payout.GetCreatorPayouts(r.Context(), userID)
	if err != nil {
		h.logger.Error("get my payouts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get payouts")
		return
	}

	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) ListAllPayouts(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	data, total, err := h.services.Payout.ListAllPayouts(r.Context(), status, limit, offset)
	if err != nil {
		h.logger.Error("list all payouts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list payouts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  data,
		"total": total,
	})
}
