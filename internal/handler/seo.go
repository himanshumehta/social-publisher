package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func (h *Handler) AnalyzeSEO(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.AnalyzeSEORequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	analysis, err := h.services.SEO.AnalyzeSEO(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("analyze seo failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, analysis)
}

func (h *Handler) GetSEOAnalysis(w http.ResponseWriter, r *http.Request) {
	analysisID, ok := h.requirePathUUID(w, r, "id", "analysis id")
	if !ok {
		return
	}

	analysis, err := h.services.SEO.GetAnalysis(r.Context(), analysisID)
	if err != nil {
		h.logger.Error("get seo analysis failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, analysis)
}

func (h *Handler) ListSEOAnalyses(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	analyses, err := h.services.SEO.ListAnalysesByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list seo analyses failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list analyses")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(analyses)})
}

// ── Keywords ──

func (h *Handler) TrackSEOKeyword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.TrackKeywordRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	kw, err := h.services.SEO.TrackKeyword(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("track keyword failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, kw)
}

func (h *Handler) ListSEOKeywords(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	keywords, err := h.services.SEO.ListKeywords(r.Context(), userID)
	if err != nil {
		h.logger.Error("list keywords failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list keywords")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(keywords)})
}

func (h *Handler) DeleteSEOKeyword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	keywordID, ok := h.requirePathUUID(w, r, "id", "keyword id")
	if !ok {
		return
	}

	if err := h.services.SEO.DeleteKeyword(r.Context(), userID, keywordID); err != nil {
		h.logger.Error("delete keyword failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Content Suggestions ──

func (h *Handler) CreateContentSuggestion(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateContentSuggestionRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	suggestion, err := h.services.SEO.CreateSuggestion(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create content suggestion failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, suggestion)
}

func (h *Handler) ListContentSuggestions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	includeDismissed := r.URL.Query().Get("include_dismissed") == "true"

	suggestions, err := h.services.SEO.ListSuggestions(r.Context(), userID, includeDismissed)
	if err != nil {
		h.logger.Error("list content suggestions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list suggestions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(suggestions)})
}

func (h *Handler) DismissContentSuggestion(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	suggestionID, ok := h.requirePathUUID(w, r, "id", "suggestion id")
	if !ok {
		return
	}

	if err := h.services.SEO.DismissSuggestion(r.Context(), userID, suggestionID); err != nil {
		h.logger.Error("dismiss suggestion failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

// ── Competitors ──

func (h *Handler) AddCompetitor(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.AddCompetitorRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	competitor, err := h.services.SEO.AddCompetitor(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("add competitor failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, competitor)
}

func (h *Handler) ListCompetitors(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	competitors, err := h.services.SEO.ListCompetitors(r.Context(), userID)
	if err != nil {
		h.logger.Error("list competitors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list competitors")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(competitors)})
}

func (h *Handler) RemoveCompetitor(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	competitorID, ok := h.requirePathUUID(w, r, "id", "competitor id")
	if !ok {
		return
	}

	if err := h.services.SEO.RemoveCompetitor(r.Context(), userID, competitorID); err != nil {
		h.logger.Error("remove competitor failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
