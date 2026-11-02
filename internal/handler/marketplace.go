package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/service"
)

// ── Creator: Job CRUD ───────────────────────────────────────────────────

func (h *Handler) CreateEditingJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.CreateEditingJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.Marketplace.CreateJob(r.Context(), userID, projectID, req)
	if err != nil {
		h.logger.Error("create editing job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (h *Handler) GetEditingJob(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	// For this route we look up the job by project ID. The project can have
	// at most one active (non-cancelled) editing job at a time.
	job, err := h.services.Marketplace.GetJobByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("get editing job failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) UpdateEditingJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	var req service.UpdateEditingJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	job, err := h.services.Marketplace.UpdateJob(r.Context(), jobID, userID, req)
	if err != nil {
		h.logger.Error("update editing job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) CancelEditingJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	if err := h.services.Marketplace.CancelJob(r.Context(), jobID, userID); err != nil {
		h.logger.Error("cancel editing job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ── Creator: Application management ─────────────────────────────────────

func (h *Handler) ListJobApplications(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	apps, err := h.services.Marketplace.ListApplications(r.Context(), jobID, userID)
	if err != nil {
		h.logger.Error("list job applications failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": apps})
}

func (h *Handler) AcceptApplication(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	appID, ok := h.requirePathUUID(w, r, "id", "application id")
	if !ok {
		return
	}

	assignment, err := h.services.Marketplace.AcceptApplication(r.Context(), appID, userID)
	if err != nil {
		h.logger.Error("accept application failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, assignment)
}

func (h *Handler) RejectApplication(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	appID, ok := h.requirePathUUID(w, r, "id", "application id")
	if !ok {
		return
	}

	if err := h.services.Marketplace.RejectApplication(r.Context(), appID, userID); err != nil {
		h.logger.Error("reject application failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// ── Creator: Delivery management ────────────────────────────────────────

func (h *Handler) AcceptDelivery(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	assignmentID, ok := h.requirePathUUID(w, r, "id", "assignment id")
	if !ok {
		return
	}

	if err := h.services.Marketplace.AcceptDelivery(r.Context(), assignmentID, userID); err != nil {
		h.logger.Error("accept delivery failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

func (h *Handler) RequestChanges(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	assignmentID, ok := h.requirePathUUID(w, r, "id", "assignment id")
	if !ok {
		return
	}

	var req service.RequestChangesRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Marketplace.RequestChanges(r.Context(), assignmentID, userID, req.Notes); err != nil {
		h.logger.Error("request changes failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "changes_requested"})
}

// ── Editor: Job board & applications ────────────────────────────────────

func (h *Handler) ListOpenJobs(w http.ResponseWriter, r *http.Request) {
	limit := 20
	offset := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			limit = v
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			offset = v
		}
	}

	jobs, total, err := h.services.Marketplace.ListOpenJobs(r.Context(), limit, offset)
	if err != nil {
		h.logger.Error("list open jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  jobs,
		"total": total,
	})
}

func (h *Handler) ApplyForJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	var req service.ApplyForJobRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	app, err := h.services.Marketplace.Apply(r.Context(), jobID, userID, req)
	if err != nil {
		h.logger.Error("apply for job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (h *Handler) WithdrawApplication(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	appID, ok := h.requirePathUUID(w, r, "id", "application id")
	if !ok {
		return
	}

	if err := h.services.Marketplace.WithdrawApplication(r.Context(), appID, userID); err != nil {
		h.logger.Error("withdraw application failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}

// ── Editor: Assignments ─────────────────────────────────────────────────

func (h *Handler) ListMyAssignments(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	assignments, err := h.services.Marketplace.ListMyAssignments(r.Context(), userID)
	if err != nil {
		h.logger.Error("list my assignments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": assignments})
}

func (h *Handler) DeliverWork(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	assignmentID, ok := h.requirePathUUID(w, r, "id", "assignment id")
	if !ok {
		return
	}

	var req service.DeliverWorkRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Marketplace.DeliverWork(r.Context(), assignmentID, userID, req.Notes); err != nil {
		h.logger.Error("deliver work failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}

// ── Shared: Ratings & Config ────────────────────────────────────────────

func (h *Handler) RateUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	assignmentID, ok := h.requirePathUUID(w, r, "id", "assignment id")
	if !ok {
		return
	}

	var req service.RateUserRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Marketplace.RateUser(r.Context(), assignmentID, userID, req.RatedUser, req.Rating, req.Review); err != nil {
		h.logger.Error("rate user failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "rated"})
}

func (h *Handler) GetEditorStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	stats, err := h.services.Marketplace.GetEditorStats(r.Context(), userID)
	if err != nil {
		h.logger.Error("get editor stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get editor stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetPriceConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.services.Marketplace.GetPriceConfig(r.Context())
	if err != nil {
		h.logger.Error("get price config failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get price config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": config})
}
