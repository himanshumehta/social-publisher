package handler

import (
	"net/http"
	"strconv"

	"github.com/himanshumehta/social-publisher/internal/service"
)

// ListProjectAssets returns all assets for a project grouped by category.
func (h *Handler) ListProjectAssets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	trees, err := h.services.ProjectAsset.ListAssets(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list project assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}

	writeJSON(w, http.StatusOK, trees)
}

// SearchProjectAssets searches assets by filename or type.
func (h *Handler) SearchProjectAssets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	fileType := r.URL.Query().Get("type")

	assets, err := h.services.ProjectAsset.SearchAssets(r.Context(), projectID, q, fileType)
	if err != nil {
		h.logger.Error("search project assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to search assets")
		return
	}

	writeJSON(w, http.StatusOK, assets)
}

// CreateProjectAsset registers a new asset for a project.
func (h *Handler) CreateProjectAsset(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.CreateAssetRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	asset, err := h.services.ProjectAsset.CreateAsset(r.Context(), projectID, userID, req)
	if err != nil {
		h.logger.Error("create project asset", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, asset)
}

// DeleteProjectAsset soft-deletes a single asset.
func (h *Handler) DeleteProjectAsset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}
	assetID, ok := h.requirePathUUID(w, r, "assetId", "asset id")
	if !ok {
		return
	}

	if err := h.services.ProjectAsset.DeleteAsset(r.Context(), projectID, assetID); err != nil {
		h.logger.Error("delete project asset", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete asset")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// BulkDeleteProjectAssets soft-deletes multiple assets.
func (h *Handler) BulkDeleteProjectAssets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.BulkDeleteAssetsRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if len(req.AssetIDs) == 0 {
		writeError(w, http.StatusBadRequest, "asset_ids required")
		return
	}

	count, err := h.services.ProjectAsset.BulkDeleteAssets(r.Context(), projectID, req.AssetIDs)
	if err != nil {
		h.logger.Error("bulk delete assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete assets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"deleted": count})
}

// RestoreProjectAsset restores a soft-deleted asset.
func (h *Handler) RestoreProjectAsset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}
	assetID, ok := h.requirePathUUID(w, r, "assetId", "asset id")
	if !ok {
		return
	}

	if err := h.services.ProjectAsset.RestoreAsset(r.Context(), projectID, assetID); err != nil {
		h.logger.Error("restore project asset", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to restore asset")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// RenameProjectAsset renames an asset file.
func (h *Handler) RenameProjectAsset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}
	assetID, ok := h.requirePathUUID(w, r, "assetId", "asset id")
	if !ok {
		return
	}

	var req service.RenameAssetRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.ProjectAsset.RenameAsset(r.Context(), projectID, assetID, req); err != nil {
		h.logger.Error("rename project asset", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

// MoveProjectAsset changes asset category/subcategory.
func (h *Handler) MoveProjectAsset(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}
	assetID, ok := h.requirePathUUID(w, r, "assetId", "asset id")
	if !ok {
		return
	}

	var req service.MoveAssetRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.ProjectAsset.MoveAsset(r.Context(), projectID, assetID, req); err != nil {
		h.logger.Error("move project asset", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "moved"})
}

// ListDeletedProjectAssets returns soft-deleted assets (trash).
func (h *Handler) ListDeletedProjectAssets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	assets, err := h.services.ProjectAsset.ListDeletedAssets(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list deleted assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list deleted assets")
		return
	}

	writeJSON(w, http.StatusOK, assets)
}

// PurgeDeletedProjectAssets permanently removes old deleted assets.
func (h *Handler) PurgeDeletedProjectAssets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	count, err := h.services.ProjectAsset.PurgeDeleted(r.Context(), projectID)
	if err != nil {
		h.logger.Error("purge deleted assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to purge")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"purged": count})
}

// GetProjectKitStatus returns the kit generation status.
func (h *Handler) GetProjectKitStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	status, err := h.services.ProjectAsset.GetKitStatus(r.Context(), projectID)
	if err != nil {
		h.logger.Error("get kit status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get kit status")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// UpdateProjectKitStatus updates the kit generation status.
func (h *Handler) UpdateProjectKitStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.UpdateKitStatusRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.ProjectAsset.UpdateKitStatus(r.Context(), projectID, req); err != nil {
		h.logger.Error("update kit status", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListProjectChangelog returns asset changelog entries.
func (h *Handler) ListProjectChangelog(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	entries, err := h.services.ProjectAsset.ListChangelog(r.Context(), projectID, limit)
	if err != nil {
		h.logger.Error("list changelog", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list changelog")
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

// AddProjectChangelog adds a changelog entry.
func (h *Handler) AddProjectChangelog(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	var req service.AddChangelogRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.ProjectAsset.AddChangelog(r.Context(), projectID, req); err != nil {
		h.logger.Error("add changelog", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to add changelog")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}
