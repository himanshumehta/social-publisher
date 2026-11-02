package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Organization CRUD ───────────────────────────────────────────────────

func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateOrgRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	org, err := h.services.Org.CreateOrg(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create organization failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	org, err := h.services.Org.GetOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("get organization failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, org)
}

func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	var req model.CreateOrgRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	org, err := h.services.Org.UpdateOrg(r.Context(), userID, orgID, &req)
	if err != nil {
		h.logger.Error("update organization failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, org)
}

func (h *Handler) ListMyOrganizations(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgs, err := h.services.Org.ListUserOrgs(r.Context(), userID)
	if err != nil {
		h.logger.Error("list organizations failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list organizations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(orgs)})
}

// ── Member management ───────────────────────────────────────────────────

func (h *Handler) InviteOrgMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	var req model.InviteOrgMemberRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	member, err := h.services.Org.InviteMember(r.Context(), userID, orgID, &req)
	if err != nil {
		h.logger.Error("invite org member failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) AcceptOrgInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	membershipID, ok := h.requirePathUUID(w, r, "id", "membership id")
	if !ok {
		return
	}

	if err := h.services.Org.AcceptOrgInvite(r.Context(), userID, membershipID); err != nil {
		h.logger.Error("accept org invite failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (h *Handler) RemoveOrgMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	memberID, ok := h.requirePathUUID(w, r, "memberId", "member id")
	if !ok {
		return
	}

	if err := h.services.Org.RemoveMember(r.Context(), userID, orgID, memberID); err != nil {
		h.logger.Error("remove org member failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) UpdateOrgMemberRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	memberID, ok := h.requirePathUUID(w, r, "memberId", "member id")
	if !ok {
		return
	}

	var req model.UpdateOrgMemberRoleRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Org.UpdateMemberRole(r.Context(), userID, orgID, memberID, req.Role); err != nil {
		h.logger.Error("update org member role failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	members, err := h.services.Org.ListMembers(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list org members failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(members)})
}

// ── Dashboard & Projects ────────────────────────────────────────────────

func (h *Handler) GetOrgDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	dash, err := h.services.Org.GetDashboard(r.Context(), userID, orgID)
	if err != nil {
		h.logger.Error("get org dashboard failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dash)
}

func (h *Handler) ListOrgProjects(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	projects, total, err := h.services.Org.ListOrgProjects(r.Context(), orgID, page, pageSize)
	if err != nil {
		h.logger.Error("list org projects failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  safeSlice(projects),
		"total": total,
	})
}

// ── Context switching ───────────────────────────────────────────────────

func (h *Handler) SwitchOrg(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	orgID, ok := h.requirePathUUID(w, r, "id", "organization id")
	if !ok {
		return
	}

	if err := h.services.Org.SwitchOrg(r.Context(), userID, orgID); err != nil {
		h.logger.Error("switch org failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "switched"})
}
