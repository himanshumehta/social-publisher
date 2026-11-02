package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateAIJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.CreateAIJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.AIJob.Create(r.Context(), projectID, userID, req)
	if err != nil {
		h.logger.Error("create ai job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) ListAIJobs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	jobs, err := h.services.AIJob.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list ai jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list ai jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

func (h *Handler) GetAIJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	job, err := h.services.AIJob.GetJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("get ai job failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Also fetch assets for the job
	assets, err := h.services.AIJob.ListAssets(r.Context(), jobID)
	if err != nil {
		h.logger.Error("list ai assets failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job":    job,
		"assets": assets,
	})
}

func (h *Handler) CancelAIJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	if err := h.services.AIJob.Cancel(r.Context(), jobID); err != nil {
		h.logger.Error("cancel ai job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) GetCredits(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	credits, err := h.services.Credits.GetBalance(r.Context(), userID)
	if err != nil {
		h.logger.Error("get credits failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get credits")
		return
	}

	writeJSON(w, http.StatusOK, credits)
}

func (h *Handler) ListCreditTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	txns, err := h.services.Credits.ListTransactions(r.Context(), userID, limit)
	if err != nil {
		h.logger.Error("list credit transactions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": txns})
}
