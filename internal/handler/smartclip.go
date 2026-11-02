package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func (h *Handler) CreateSmartClipJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateSmartClipJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.SmartClip.CreateJob(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create smart clip job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) GetSmartClipJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	job, err := h.services.SmartClip.GetJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("get smart clip job failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) ListSmartClipJobsByProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	jobs, err := h.services.SmartClip.ListByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list smart clip jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list smart clip jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(jobs)})
}

func (h *Handler) ListSmartClips(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	clips, err := h.services.SmartClip.ListClips(r.Context(), jobID)
	if err != nil {
		h.logger.Error("list smart clips failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list clips")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(clips)})
}

func (h *Handler) ListClipMoments(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	moments, err := h.services.SmartClip.ListMoments(r.Context(), jobID)
	if err != nil {
		h.logger.Error("list clip moments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list moments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(moments)})
}

func (h *Handler) GetTopSmartClips(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	clips, err := h.services.SmartClip.GetTopClips(r.Context(), jobID, 5)
	if err != nil {
		h.logger.Error("get top clips failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get top clips")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(clips)})
}

func (h *Handler) CancelSmartClipJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	if err := h.services.SmartClip.Cancel(r.Context(), userID, jobID); err != nil {
		h.logger.Error("cancel smart clip job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) EstimateSmartClipCredits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClipCount int `json:"clip_count"`
	}
	if !h.requireJSON(w, r, &req) {
		return
	}

	credits := h.services.SmartClip.EstimateCredits(req.ClipCount)
	writeJSON(w, http.StatusOK, map[string]any{"credits": credits})
}
