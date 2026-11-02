package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) CreateEditorAlignment(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.CreateAlignmentRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	alignment, err := h.services.EditorAlignment.Create(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("create editor alignment failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, alignment)
}

func (h *Handler) ListEditorAlignmentsByCreator(w http.ResponseWriter, r *http.Request) {
	creatorID, ok := h.requirePathUUID(w, r, "creatorId", "creator id")
	if !ok {
		return
	}

	alignments, err := h.services.EditorAlignment.ListByCreator(r.Context(), creatorID)
	if err != nil {
		h.logger.Error("list editor alignments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alignments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": alignments})
}

func (h *Handler) ListEditorAlignmentsByEditor(w http.ResponseWriter, r *http.Request) {
	editorID, ok := h.requirePathUUID(w, r, "editorId", "editor id")
	if !ok {
		return
	}

	alignments, err := h.services.EditorAlignment.ListByEditor(r.Context(), editorID)
	if err != nil {
		h.logger.Error("list editor alignments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list alignments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": alignments})
}

func (h *Handler) DeleteEditorAlignment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "alignment id")
	if !ok {
		return
	}

	if err := h.services.EditorAlignment.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete editor alignment failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ListMyAlignedEditors(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	alignments, err := h.services.EditorAlignment.ListByCreator(r.Context(), userID)
	if err != nil {
		h.logger.Error("list my aligned editors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list aligned editors")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": alignments})
}
