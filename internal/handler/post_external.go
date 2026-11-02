package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/model"
)

// ExternalCreatePost handles POST /api/v1/external/posts (API key auth)
func (h *Handler) ExternalCreatePost(w http.ResponseWriter, r *http.Request) {
	apiKeyCtx := middleware.GetAPIKey(r)
	if apiKeyCtx == nil {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}

	if !apiKeyCtx.HasScope("posts:create") {
		writeError(w, http.StatusForbidden, "API key lacks posts:create scope")
		return
	}

	var req model.CreatePostRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	post, err := h.services.Post.Create(r.Context(), apiKeyCtx.UserID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      post.ID,
		"status":  post.Status,
		"message": "Post created and queued for processing",
	})
}

// ExternalGetPost handles GET /api/v1/external/posts/{id} (API key auth)
func (h *Handler) ExternalGetPost(w http.ResponseWriter, r *http.Request) {
	apiKeyCtx := middleware.GetAPIKey(r)
	if apiKeyCtx == nil {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}

	if !apiKeyCtx.HasScope("posts:read") {
		writeError(w, http.StatusForbidden, "API key lacks posts:read scope")
		return
	}

	postID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	post, err := h.services.Post.GetByID(r.Context(), postID, apiKeyCtx.UserID, false)
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, post)
}
