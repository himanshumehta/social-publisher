package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Newsletter CRUD ──

func (h *Handler) CreateNewsletter(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req model.CreateNewsletterRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	nl, err := h.services.Newsletter.Create(r.Context(), userID, &req)
	if err != nil {
		h.logger.Error("create newsletter failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, nl)
}

func (h *Handler) GetNewsletter(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	nl, err := h.services.Newsletter.Get(r.Context(), nlID)
	if err != nil {
		h.logger.Error("get newsletter failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, nl)
}

func (h *Handler) ListNewsletters(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	newsletters, err := h.services.Newsletter.ListByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("list newsletters failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list newsletters")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(newsletters)})
}

func (h *Handler) UpdateNewsletter(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	var req model.UpdateNewsletterRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	nl, err := h.services.Newsletter.Update(r.Context(), userID, nlID, &req)
	if err != nil {
		h.logger.Error("update newsletter failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, nl)
}

func (h *Handler) DeleteNewsletter(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	if err := h.services.Newsletter.Delete(r.Context(), userID, nlID); err != nil {
		h.logger.Error("delete newsletter failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Subscribers ──

func (h *Handler) AddNewsletterSubscriber(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	var req model.AddSubscriberRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	sub, err := h.services.Newsletter.AddSubscriber(r.Context(), nlID, &req)
	if err != nil {
		h.logger.Error("add subscriber failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sub)
}

func (h *Handler) BulkAddSubscribers(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	var req model.BulkAddSubscribersRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	count, err := h.services.Newsletter.BulkAddSubscribers(r.Context(), nlID, &req)
	if err != nil {
		h.logger.Error("bulk add subscribers failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"added": count})
}

func (h *Handler) ListNewsletterSubscribers(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")

	subs, err := h.services.Newsletter.ListSubscribers(r.Context(), nlID, status)
	if err != nil {
		h.logger.Error("list subscribers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list subscribers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(subs)})
}

func (h *Handler) UnsubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if !h.requireJSON(w, r, &req) {
		return
	}

	if err := h.services.Newsletter.Unsubscribe(r.Context(), nlID, req.Email); err != nil {
		h.logger.Error("unsubscribe failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

func (h *Handler) RemoveNewsletterSubscriber(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	subID, ok := h.requirePathUUID(w, r, "subId", "subscriber id")
	if !ok {
		return
	}

	if err := h.services.Newsletter.RemoveSubscriber(r.Context(), nlID, subID); err != nil {
		h.logger.Error("remove subscriber failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ── Issues ──

func (h *Handler) CreateNewsletterIssue(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	var req model.CreateIssueRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	issue, err := h.services.Newsletter.CreateIssue(r.Context(), nlID, &req)
	if err != nil {
		h.logger.Error("create issue failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, issue)
}

func (h *Handler) GetNewsletterIssue(w http.ResponseWriter, r *http.Request) {
	issueID, ok := h.requirePathUUID(w, r, "id", "issue id")
	if !ok {
		return
	}

	issue, err := h.services.Newsletter.GetIssue(r.Context(), issueID)
	if err != nil {
		h.logger.Error("get issue failed", "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) ListNewsletterIssues(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	issues, err := h.services.Newsletter.ListIssues(r.Context(), nlID)
	if err != nil {
		h.logger.Error("list issues failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": safeSlice(issues)})
}

func (h *Handler) SendNewsletterIssue(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	issueID, ok := h.requirePathUUID(w, r, "id", "issue id")
	if !ok {
		return
	}

	if err := h.services.Newsletter.SendIssue(r.Context(), userID, issueID); err != nil {
		h.logger.Error("send issue failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) DeleteNewsletterIssue(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	issueID, ok := h.requirePathUUID(w, r, "issueId", "issue id")
	if !ok {
		return
	}

	if err := h.services.Newsletter.DeleteIssue(r.Context(), nlID, issueID); err != nil {
		h.logger.Error("delete issue failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Stats ──

func (h *Handler) GetNewsletterStats(w http.ResponseWriter, r *http.Request) {
	nlID, ok := h.requirePathUUID(w, r, "id", "newsletter id")
	if !ok {
		return
	}

	stats, err := h.services.Newsletter.GetStats(r.Context(), nlID)
	if err != nil {
		h.logger.Error("get newsletter stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
