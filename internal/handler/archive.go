package handler

import (
	"net/http"
)

func (h *Handler) ArchiveProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	task, err := h.services.Archive.InitiateArchive(r.Context(), userID, projectID)
	if err != nil {
		h.logger.Error("initiate archive failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) GetArchiveStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	task, err := h.services.Archive.GetArchiveStatus(r.Context(), projectID)
	if err != nil {
		h.logger.Error("get archive status failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get archive status")
		return
	}

	if task == nil {
		writeJSON(w, http.StatusOK, map[string]any{"archive_task": nil})
		return
	}

	writeJSON(w, http.StatusOK, task)
}
