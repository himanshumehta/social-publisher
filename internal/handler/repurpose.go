package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// CreateRepurposeJob handles POST /api/v1/posts/{id}/repurpose
func (h *Handler) CreateRepurposeJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	sourcePostID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	var body struct {
		TargetPlatforms []string                `json:"target_platforms"`
		Strategy        string                  `json:"strategy"`
		Options         *model.RepurposeOptions `json:"options,omitempty"`
	}
	if !h.requireJSON(w, r, &body) {
		return
	}

	req := &model.CreateRepurposeRequest{
		SourcePostID:    sourcePostID,
		TargetPlatforms: body.TargetPlatforms,
		Strategy:        body.Strategy,
		Options:         body.Options,
	}

	job, err := h.services.Repurpose.CreateJob(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("create repurpose job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

// GetRepurposeJob handles GET /api/v1/repurpose-jobs/{id}
func (h *Handler) GetRepurposeJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "repurpose job id")
	if !ok {
		return
	}

	job, err := h.services.Repurpose.GetJob(r.Context(), userID, jobID)
	if err != nil {
		h.logger.Error("get repurpose job failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// ListRepurposeJobs handles GET /api/v1/repurpose-jobs
func (h *Handler) ListRepurposeJobs(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobs, err := h.services.Repurpose.ListJobs(r.Context(), userID)
	if err != nil {
		h.logger.Error("list repurpose jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list repurpose jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(jobs)})
}

// GetRepurposeOutputs handles GET /api/v1/repurpose-jobs/{id}/outputs
func (h *Handler) GetRepurposeOutputs(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "repurpose job id")
	if !ok {
		return
	}

	outputs, err := h.services.Repurpose.GetOutputs(r.Context(), userID, jobID)
	if err != nil {
		h.logger.Error("get repurpose outputs failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(outputs)})
}

// AcceptRepurposeOutput handles POST /api/v1/repurpose-outputs/{id}/accept
func (h *Handler) AcceptRepurposeOutput(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	outputID, ok := h.requirePathUUID(w, r, "id", "repurpose output id")
	if !ok {
		return
	}

	newPostID, err := h.services.Repurpose.AcceptOutput(r.Context(), userID, outputID)
	if err != nil {
		h.logger.Error("accept repurpose output failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":      "accepted",
		"new_post_id": newPostID,
	})
}
