package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/model"
)

// ListProjectPosts handles GET /api/v1/projects/{id}/posts
func (h *Handler) ListProjectPosts(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project ID")
	if !ok {
		return
	}

	posts, err := h.services.Post.ListByProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": posts,
	})
}

// CreatePost handles POST /api/v1/posts
func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreatePostRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	post, err := h.services.Post.Create(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, post)
}

// GetPost handles GET /api/v1/posts/{id}
func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	isAdmin := role == "admin" || role == "super_admin"

	post, err := h.services.Post.GetByID(r.Context(), postID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, post)
}

// ListPosts handles GET /api/v1/posts
func (h *Handler) ListPosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	isAdmin := role == "admin" || role == "super_admin"

	page := 1
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	pageSize := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 && v <= 100 {
		pageSize = v
	}

	filter := &model.PostListFilter{
		Status:    r.URL.Query().Get("status"),
		Platform:  r.URL.Query().Get("platform"),
		Search:    r.URL.Query().Get("search"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
		Page:      page,
		PageSize:  pageSize,
	}

	items, total, err := h.services.Post.List(r.Context(), filter, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"posts":     items,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

// UpdatePost handles PUT /api/v1/posts/{id}
func (h *Handler) UpdatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	var req model.UpdatePostRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	role := middleware.RoleFromContext(r.Context())
	isAdmin := role == "admin" || role == "super_admin"

	post, err := h.services.Post.Update(r.Context(), postID, userID, isAdmin, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, post)
}

// DeletePost handles DELETE /api/v1/posts/{id}
func (h *Handler) DeletePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	role := middleware.RoleFromContext(r.Context())
	isAdmin := role == "admin" || role == "super_admin"

	if err := h.services.Post.Delete(r.Context(), postID, userID, isAdmin); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RegeneratePost handles POST /api/v1/posts/{id}/regenerate
func (h *Handler) RegeneratePost(w http.ResponseWriter, r *http.Request) {
	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	if err := h.services.Post.Regenerate(r.Context(), postID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "regenerating"})
}

// SubmitPost handles POST /api/v1/posts/{id}/submit
func (h *Handler) SubmitPost(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	postID, ok := h.requirePathUUID(w, r, "id", "post ID")
	if !ok {
		return
	}

	if err := h.services.Post.Submit(r.Context(), postID, userID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "submitted"})
}
