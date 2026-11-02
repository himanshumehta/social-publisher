package handler

import (
	"net/http"
	"strconv"
)

// GetViralityScore handles GET /api/v1/posts/{id}/virality-score
// Returns the existing score, or triggers scoring if none exists.
func (h *Handler) GetViralityScore(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	postID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	// Try to get existing score first.
	score, err := h.services.Virality.GetScore(r.Context(), userID, postID)
	if err != nil {
		h.logger.Error("get virality score failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get virality score")
		return
	}

	// If no score exists, trigger scoring on demand.
	if score == nil {
		score, err = h.services.Virality.ScorePost(r.Context(), userID, postID)
		if err != nil {
			h.logger.Error("score post failed", "error", err)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, score)
}

// GetViralitySuggestions handles GET /api/v1/posts/{id}/suggestions
func (h *Handler) GetViralitySuggestions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	postID, ok := h.requirePathUUID(w, r, "id", "post id")
	if !ok {
		return
	}

	suggestions, err := h.services.Virality.GetSuggestions(r.Context(), userID, postID)
	if err != nil {
		h.logger.Error("get suggestions failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get suggestions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": safeSlice(suggestions),
	})
}

// GetTrendingHashtags handles GET /api/v1/trending-hashtags?platform=twitter&limit=20
func (h *Handler) GetTrendingHashtags(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	platform := r.URL.Query().Get("platform")
	if platform == "" {
		writeError(w, http.StatusBadRequest, "platform query parameter is required")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	tags, err := h.services.Virality.GetTrendingHashtags(r.Context(), platform, limit)
	if err != nil {
		h.logger.Error("get trending hashtags failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get trending hashtags")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": safeSlice(tags),
	})
}
