package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── CreateApprovalWorkflow ──────────────────────────────────────────────

// CreateApprovalWorkflow handles POST /api/v1/workflows
func (h *Handler) CreateApprovalWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateWorkflowRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	wf, err := h.services.Approval.CreateWorkflow(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create workflow failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, wf)
}

// ── GetApprovalWorkflow ─────────────────────────────────────────────────

// GetApprovalWorkflow handles GET /api/v1/workflows/{id}
func (h *Handler) GetApprovalWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	wfID, ok := h.requirePathUUID(w, r, "id", "workflow id")
	if !ok {
		return
	}

	wf, err := h.services.Approval.GetWorkflow(r.Context(), userID, wfID)
	if err != nil {
		h.logger.Error("get workflow failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, wf)
}

// ── ListApprovalWorkflows ───────────────────────────────────────────────

// ListApprovalWorkflows handles GET /api/v1/workflows
func (h *Handler) ListApprovalWorkflows(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	workflows, err := h.services.Approval.ListWorkflows(r.Context(), userID)
	if err != nil {
		h.logger.Error("list workflows failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(workflows))
}

// ── DeleteApprovalWorkflow ──────────────────────────────────────────────

// DeleteApprovalWorkflow handles DELETE /api/v1/workflows/{id}
func (h *Handler) DeleteApprovalWorkflow(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	wfID, ok := h.requirePathUUID(w, r, "id", "workflow id")
	if !ok {
		return
	}

	if err := h.services.Approval.DeleteWorkflow(r.Context(), userID, wfID); err != nil {
		h.logger.Error("delete workflow failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── SubmitForApproval ───────────────────────────────────────────────────

// SubmitForApproval handles POST /api/v1/posts/{id}/submit-approval
func (h *Handler) SubmitForApproval(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	var req model.SubmitApprovalRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	ar, err := h.services.Approval.SubmitForApproval(r.Context(), userID, postID, &req)
	if err != nil {
		h.logger.Error("submit for approval failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, ar)
}

// ── TakeApprovalAction ──────────────────────────────────────────────────

// TakeApprovalAction handles POST /api/v1/approvals/{id}/action
func (h *Handler) TakeApprovalAction(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	requestID, ok := h.requirePathUUID(w, r, "id", "approval request id")
	if !ok {
		return
	}

	var req model.ApprovalActionRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	ar, err := h.services.Approval.TakeAction(r.Context(), userID, requestID, &req)
	if err != nil {
		h.logger.Error("take approval action failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ar)
}

// ── GetApprovalStatus ───────────────────────────────────────────────────

// GetApprovalStatus handles GET /api/v1/posts/{id}/approval-status
func (h *Handler) GetApprovalStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	ar, err := h.services.Approval.GetApprovalStatus(r.Context(), postID)
	if err != nil {
		h.logger.Error("get approval status failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ar)
}

// ── ListPendingApprovals ────────────────────────────────────────────────

// ListPendingApprovals handles GET /api/v1/approvals/pending
func (h *Handler) ListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	approvals, err := h.services.Approval.ListPendingApprovals(r.Context(), userID)
	if err != nil {
		h.logger.Error("list pending approvals failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(approvals))
}
