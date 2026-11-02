package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Brand Kit Handlers ──────────────────────────────────────────────────

// CreateBrandKit -- POST /api/v1/brand-kits
func (h *Handler) CreateBrandKit(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateBrandKitRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	kit, err := h.services.BrandKit.CreateKit(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create brand kit failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, kit)
}

// GetBrandKit -- GET /api/v1/brand-kits/{id}
func (h *Handler) GetBrandKit(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	kit, err := h.services.BrandKit.GetKit(r.Context(), userID, kitID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, kit)
}

// ListBrandKits -- GET /api/v1/brand-kits
func (h *Handler) ListBrandKits(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	kits, err := h.services.BrandKit.ListKits(r.Context(), userID)
	if err != nil {
		h.logger.Error("list brand kits failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list brand kits")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": safeSlice(kits),
	})
}

// UpdateBrandKit -- PUT /api/v1/brand-kits/{id}
func (h *Handler) UpdateBrandKit(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	var req model.UpdateBrandKitRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	kit, err := h.services.BrandKit.UpdateKit(r.Context(), userID, kitID, &req)
	if err != nil {
		h.logger.Error("update brand kit failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, kit)
}

// DeleteBrandKit -- DELETE /api/v1/brand-kits/{id}
func (h *Handler) DeleteBrandKit(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	if err := h.services.BrandKit.DeleteKit(r.Context(), userID, kitID); err != nil {
		h.logger.Error("delete brand kit failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// SetDefaultBrandKit -- POST /api/v1/brand-kits/{id}/default
func (h *Handler) SetDefaultBrandKit(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	if err := h.services.BrandKit.SetDefault(r.Context(), userID, kitID); err != nil {
		h.logger.Error("set default brand kit failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "default updated"})
}

// ── Template Handlers ───────────────────────────────────────────────────

// CreateTemplate -- POST /api/v1/brand-kits/{id}/templates
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	var req model.CreateTemplateRequest
	if !h.requireJSON(w, r, &req) {
		return
	}
	req.BrandKitID = kitID

	tmpl, err := h.services.BrandKit.CreateTemplate(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create template failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

// GetTemplate -- GET /api/v1/templates/{id}
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	templateID, ok := h.requirePathUUID(w, r, "id", "template id")
	if !ok {
		return
	}

	tmpl, err := h.services.BrandKit.GetTemplate(r.Context(), userID, templateID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tmpl)
}

// ListTemplates -- GET /api/v1/brand-kits/{id}/templates
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	kitID, ok := h.requirePathUUID(w, r, "id", "brand kit id")
	if !ok {
		return
	}

	platform := r.URL.Query().Get("platform")

	templates, err := h.services.BrandKit.ListTemplates(r.Context(), userID, &kitID, platform)
	if err != nil {
		h.logger.Error("list templates failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": safeSlice(templates),
	})
}

// RenderTemplate -- POST /api/v1/templates/{id}/render
func (h *Handler) RenderTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	templateID, ok := h.requirePathUUID(w, r, "id", "template id")
	if !ok {
		return
	}

	var body struct {
		Variables map[string]string `json:"variables"`
	}
	if !h.requireJSON(w, r, &body) {
		return
	}

	req := &model.RenderTemplateRequest{
		TemplateID: templateID,
		Variables:  body.Variables,
	}

	result, err := h.services.BrandKit.RenderTemplate(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("render template failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// DeleteTemplate -- DELETE /api/v1/templates/{id}
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	templateID, ok := h.requirePathUUID(w, r, "id", "template id")
	if !ok {
		return
	}

	if err := h.services.BrandKit.DeleteTemplate(r.Context(), userID, templateID); err != nil {
		h.logger.Error("delete template failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
