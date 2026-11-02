package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── InviteMember ────────────────────────────────────────────────────────

// InviteMember handles POST /api/v1/team/members
func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.InviteMemberRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	member, err := h.services.Team.InviteMember(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("invite member failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

// ── AcceptTeamInvite ────────────────────────────────────────────────────

// AcceptTeamInvite handles POST /api/v1/team/invites/{id}/accept
func (h *Handler) AcceptTeamInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	inviteID, ok := h.requirePathUUID(w, r, "id", "invite id")
	if !ok {
		return
	}

	if err := h.services.Team.AcceptInvite(r.Context(), userID, inviteID); err != nil {
		h.logger.Error("accept invite failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// ── RemoveTeamMember ────────────────────────────────────────────────────

// RemoveTeamMember handles DELETE /api/v1/team/members/{id}
func (h *Handler) RemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	memberID, ok := h.requirePathUUID(w, r, "id", "member id")
	if !ok {
		return
	}

	if err := h.services.Team.RemoveMember(r.Context(), userID, memberID); err != nil {
		h.logger.Error("remove member failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── ListTeamMembers ─────────────────────────────────────────────────────

// ListTeamMembers handles GET /api/v1/team/members
func (h *Handler) ListTeamMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	members, err := h.services.Team.ListMembers(r.Context(), userID)
	if err != nil {
		h.logger.Error("list members failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, safeSlice(members))
}

// ── UpdateTeamMemberRole ────────────────────────────────────────────────

// UpdateTeamMemberRole handles PATCH /api/v1/team/members/{id}/role
func (h *Handler) UpdateTeamMemberRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	memberID, ok := h.requirePathUUID(w, r, "id", "member id")
	if !ok {
		return
	}

	var req model.UpdateMemberRoleRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Team.UpdateRole(r.Context(), userID, memberID, req.Role); err != nil {
		h.logger.Error("update role failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
