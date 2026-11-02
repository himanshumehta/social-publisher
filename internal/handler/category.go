package handler

import "net/http"

// ListCategories handles GET /api/v1/categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.services.Category.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list categories")
		return
	}
	writeJSON(w, http.StatusOK, categories)
}
