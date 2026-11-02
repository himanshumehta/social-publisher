package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Portfolio CRUD ──────────────────────────────────────────────────────

func (h *Handler) CreateEditorPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreatePortfolioRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	portfolio, err := h.services.Portfolio.CreatePortfolio(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create editor portfolio failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, portfolio)
}

func (h *Handler) GetEditorPortfolio(w http.ResponseWriter, r *http.Request) {
	editorID, ok := h.requirePathUUID(w, r, "id", "editor id")
	if !ok {
		return
	}

	portfolio, err := h.services.Portfolio.GetPortfolio(r.Context(), editorID)
	if err != nil {
		h.logger.Error("get editor portfolio failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, portfolio)
}

func (h *Handler) UpdateEditorPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.UpdatePortfolioRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	portfolio, err := h.services.Portfolio.UpdatePortfolio(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("update editor portfolio failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, portfolio)
}

// ── Portfolio Items ─────────────────────────────────────────────────────

func (h *Handler) AddPortfolioItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.AddPortfolioItemRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	item, err := h.services.Portfolio.AddPortfolioItem(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("add portfolio item failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) RemovePortfolioItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	itemID, ok := h.requirePathUUID(w, r, "id", "item id")
	if !ok {
		return
	}

	if err := h.services.Portfolio.RemovePortfolioItem(r.Context(), userID, itemID); err != nil {
		h.logger.Error("remove portfolio item failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── Skills ──────────────────────────────────────────────────────────────

func (h *Handler) SetEditorSkills(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.SetSkillsRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	skills, err := h.services.Portfolio.SetSkills(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("set editor skills failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(skills)})
}

// ── Editor Search ───────────────────────────────────────────────────────

func (h *Handler) SearchEditors(w http.ResponseWriter, r *http.Request) {
	filter := &model.EditorSearchFilter{
		Page:     1,
		PageSize: 20,
	}

	if s := r.URL.Query().Get("skills"); s != "" {
		filter.Skills = strings.Split(s, ",")
	}
	if s := r.URL.Query().Get("min_rating"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			filter.MinRating = v
		}
	}
	if s := r.URL.Query().Get("max_rate"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			filter.MaxRate = v
		}
	}
	if s := r.URL.Query().Get("availability"); s != "" {
		filter.Availability = s
	}
	if s := r.URL.Query().Get("page"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			filter.Page = v
		}
	}
	if s := r.URL.Query().Get("page_size"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			filter.PageSize = v
		}
	}

	results, total, err := h.services.Portfolio.SearchEditors(r.Context(), filter)
	if err != nil {
		h.logger.Error("search editors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to search editors")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  safeSlice(results),
		"total": total,
	})
}

func (h *Handler) MatchEditorsForJob(w http.ResponseWriter, r *http.Request) {
	jobID, ok := h.requirePathUUID(w, r, "id", "job id")
	if !ok {
		return
	}

	results, err := h.services.Portfolio.MatchEditorsForJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("match editors for job failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(results)})
}

// ── Marketplace Templates ───────────────────────────────────────────────

func (h *Handler) CreateMarketplaceTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateMarketplaceTemplateRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	tmpl, err := h.services.Portfolio.CreateMarketplaceTemplate(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create marketplace template failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

func (h *Handler) ListMarketplaceTemplates(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	page := 1
	pageSize := 20

	if s := r.URL.Query().Get("page"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			page = v
		}
	}
	if s := r.URL.Query().Get("page_size"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			pageSize = v
		}
	}

	templates, total, err := h.services.Portfolio.ListMarketplaceTemplates(r.Context(), category, page, pageSize)
	if err != nil {
		h.logger.Error("list marketplace templates failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  safeSlice(templates),
		"total": total,
	})
}

func (h *Handler) PurchaseMarketplaceTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	templateID, ok := h.requirePathUUID(w, r, "id", "template id")
	if !ok {
		return
	}

	tmpl, err := h.services.Portfolio.PurchaseTemplate(r.Context(), userID, templateID)
	if err != nil {
		h.logger.Error("purchase marketplace template failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tmpl)
}
