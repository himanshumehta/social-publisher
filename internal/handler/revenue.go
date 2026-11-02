package handler

import (
	"net/http"
)

func (h *Handler) GetRevenueConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.services.Revenue.GetConfig(r.Context())
	if err != nil {
		h.logger.Error("get revenue config failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get revenue config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

type updateRevenueConfigRequest struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
}

func (h *Handler) UpdateRevenueConfig(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	var req updateRevenueConfigRequest
	if !h.requireJSON(w, r, &req) {
		return
	}
	if err := h.services.Revenue.UpdateConfig(r.Context(), adminID, req.Key, req.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) GenerateMonthlyPayouts(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	count, err := h.services.Revenue.GenerateMonthlyPayouts(r.Context(), adminID)
	if err != nil {
		h.logger.Error("generate monthly payouts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate payouts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": count})
}

func (h *Handler) ApprovePayout(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	payoutID, ok := h.requirePathUUID(w, r, "id", "payout id")
	if !ok {
		return
	}
	if err := h.services.Revenue.ApprovePayout(r.Context(), payoutID, adminID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

type rejectPayoutRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) RejectPayout(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	payoutID, ok := h.requirePathUUID(w, r, "id", "payout id")
	if !ok {
		return
	}
	var req rejectPayoutRequest
	if !h.requireJSON(w, r, &req) {
		return
	}
	if err := h.services.Revenue.RejectPayout(r.Context(), payoutID, adminID, req.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}
