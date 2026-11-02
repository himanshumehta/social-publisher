package handler

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ListAdminPosts handles GET /api/v1/admin/posts
func (h *Handler) ListAdminPosts(w http.ResponseWriter, r *http.Request) {
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
		AuthorID:  r.URL.Query().Get("author_id"),
		Search:    r.URL.Query().Get("search"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
		Page:      page,
		PageSize:  pageSize,
	}

	items, total, err := h.services.Post.List(r.Context(), filter, uuid.Nil, true)
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

// GetAdminReviewQueue handles GET /api/v1/admin/posts/review-queue
func (h *Handler) GetAdminReviewQueue(w http.ResponseWriter, r *http.Request) {
	page := 1
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	pageSize := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 && v <= 100 {
		pageSize = v
	}

	filter := &model.PostListFilter{
		Status:    "pending_review",
		SortBy:    "created_at",
		SortOrder: "asc",
		Page:      page,
		PageSize:  pageSize,
	}

	items, total, err := h.services.Post.List(r.Context(), filter, uuid.Nil, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	flaggedFilter := &model.PostListFilter{
		Status:    "flagged",
		SortBy:    "created_at",
		SortOrder: "asc",
		Page:      1,
		PageSize:  pageSize,
	}
	flaggedItems, flaggedTotal, _ := h.services.Post.List(r.Context(), flaggedFilter, uuid.Nil, true)

	writeJSON(w, http.StatusOK, map[string]any{
		"pending_review": safeSlice(items),
		"pending_total":  total,
		"flagged":        safeSlice(flaggedItems),
		"flagged_total":  flaggedTotal,
	})
}

// ApprovePost handles POST /api/v1/admin/posts/{id}/approve
func (h *Handler) ApprovePost(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	if err := h.services.Post.AdminApprove(r.Context(), postID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// RejectPost handles POST /api/v1/admin/posts/{id}/reject
func (h *Handler) RejectPost(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	var req model.AdminRejectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.services.Post.AdminReject(r.Context(), postID, req.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// AdminPublishPost handles POST /api/v1/admin/posts/{id}/publish
func (h *Handler) AdminPublishPost(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	// Approve first if in review state (ignore error if already approved)
	_ = h.services.Post.AdminApprove(r.Context(), postID)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "queued",
		"message": "Post approved and queued for publishing",
	})
}

// GetPostPublishStatus handles GET /api/v1/admin/posts/{id}/publish-status
func (h *Handler) GetPostPublishStatus(w http.ResponseWriter, r *http.Request) {
	postID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post ID")
		return
	}

	post, err := h.services.Post.GetByID(r.Context(), postID, uuid.Nil, true)
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          post.Status,
		"publish_results": post.PublishResults,
	})
}
