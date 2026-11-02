package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateClipJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.CreateClipJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.ClipJob.Create(r.Context(), projectID, userID, req)
	if err != nil {
		h.logger.Error("create clip job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) ListClipJobs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	jobs, err := h.services.ClipJob.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list clip jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list clip jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

func (h *Handler) ListClipsByJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "clip job id")
	if !ok {
		return
	}

	clips, err := h.services.ClipJob.ListClipsByJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("list clips by job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list clips")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": clips})
}
