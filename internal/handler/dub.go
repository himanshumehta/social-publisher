package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func (h *Handler) CreateDubJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateDubJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.Dub.CreateJob(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create dub job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) BatchCreateDubJobs(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.BatchDubRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	jobs, err := h.services.Dub.BatchCreate(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("batch create dub jobs failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": safeSlice(jobs)})
}

func (h *Handler) GetDubJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	job, err := h.services.Dub.GetJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("get dub job failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) ListDubJobsByProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	jobs, err := h.services.Dub.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list dub jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list dub jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(jobs)})
}

func (h *Handler) ListDubOutputs(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	outputs, err := h.services.Dub.ListOutputs(r.Context(), jobID)
	if err != nil {
		h.logger.Error("list dub outputs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list dub outputs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(outputs)})
}

func (h *Handler) CancelDubJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	if err := h.services.Dub.Cancel(r.Context(), userID, jobID); err != nil {
		h.logger.Error("cancel dub job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) GetDubLanguages(w http.ResponseWriter, r *http.Request) {
	langs := h.services.Dub.GetSupportedLanguages()
	writeJSON(w, http.StatusOK, map[string]any{"data": langs})
}

func (h *Handler) EstimateDubCredits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DurationSeconds int `json:"duration_seconds"`
	}
	if !h.requireJSON(w, r, &req) {
		return
	}

	credits := h.services.Dub.EstimateCredits(req.DurationSeconds)
	writeJSON(w, http.StatusOK, map[string]any{"credits": credits})
}
