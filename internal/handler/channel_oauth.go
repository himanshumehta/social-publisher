package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/service"
)

// ---------------------------------------------------------------------------
// 1. GetChannelOAuthURL — GET /api/v1/admin/channels/{id}/oauth/{provider}/url
//    Returns the platform-specific OAuth authorization URL for connecting a
//    channel. The frontend opens this URL in a popup / redirect.
// ---------------------------------------------------------------------------

func (h *Handler) GetChannelOAuthURL(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	provider := r.PathValue("provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var authURL string

	switch strings.ToLower(provider) {
	case "google":
		authURL, err = h.services.ChannelOAuth.GetGoogleOAuthURL(r.Context(), channelID)
	case "facebook":
		authURL, err = h.services.ChannelOAuth.GetMetaOAuthURL(r.Context(), channelID)
	case "instagram":
		authURL, err = h.services.ChannelOAuth.GetInstagramBusinessLoginURL(r.Context(), channelID)
	case "threads":
		authURL, err = h.services.ChannelOAuth.GetThreadsOAuthURL(r.Context(), channelID)
	case "twitter":
		authURL, err = h.services.ChannelOAuth.GetTwitterOAuthURL(r.Context(), channelID)
	case "linkedin":
		authURL, err = h.services.ChannelOAuth.GetLinkedInOAuthURL(r.Context(), channelID)
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported provider: %s", provider))
		return
	}

	if err != nil {
		h.logger.Error("get oauth url failed", "provider", provider, "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate oauth url")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

// ---------------------------------------------------------------------------
// 2. GoogleChannelCallback — GET /api/v1/admin/channels/oauth/google/callback
//    PUBLIC endpoint (no auth middleware). Receives the redirect from Google
//    after user authorization, exchanges the code for tokens, and redirects
//    the browser back to the frontend.
// ---------------------------------------------------------------------------

func (h *Handler) GoogleChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channel, err := h.services.ChannelOAuth.HandleGoogleCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("google oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	http.Redirect(w, r, h.oauthRedirectSuccess(channel.ID, channel.ChannelType), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 3. FacebookChannelCallback — GET /api/v1/admin/channels/oauth/facebook/callback
//    PUBLIC endpoint. Meta OAuth handles both Facebook Page and associated
//    Instagram Business account. Returns up to 2 channel records.
// ---------------------------------------------------------------------------

func (h *Handler) FacebookChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channels, err := h.services.ChannelOAuth.HandleMetaCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("facebook oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	// Build comma-separated list of connected channel IDs for the frontend.
	ids := make([]string, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.ID.String())
	}

	// Determine channel type from first channel for routing.
	channelType := "system"
	if len(channels) > 0 {
		channelType = channels[0].ChannelType
	}
	page := "/admin/channels"
	if channelType == "creator" {
		page = "/settings/channels"
	}
	redirectURL := fmt.Sprintf("%s%s?connected=%s", h.cfg.App.FrontendURL, page, strings.Join(ids, ","))
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 3b. InstagramChannelCallback — GET /api/v1/admin/channels/oauth/instagram/callback
//     PUBLIC endpoint. Instagram Business Login flow.
// ---------------------------------------------------------------------------

func (h *Handler) InstagramChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channel, err := h.services.ChannelOAuth.HandleInstagramBusinessLoginCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("instagram oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	http.Redirect(w, r, h.oauthRedirectSuccess(channel.ID, channel.ChannelType), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 4. ThreadsChannelCallback — GET /api/v1/admin/channels/oauth/threads/callback
//    PUBLIC endpoint.
// ---------------------------------------------------------------------------

func (h *Handler) ThreadsChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channel, err := h.services.ChannelOAuth.HandleThreadsCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("threads oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	http.Redirect(w, r, h.oauthRedirectSuccess(channel.ID, channel.ChannelType), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 5. TwitterChannelCallback — GET /api/v1/admin/channels/oauth/twitter/callback
//    PUBLIC endpoint.
// ---------------------------------------------------------------------------

func (h *Handler) TwitterChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channel, err := h.services.ChannelOAuth.HandleTwitterCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("twitter oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	http.Redirect(w, r, h.oauthRedirectSuccess(channel.ID, channel.ChannelType), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 6. LinkedInChannelCallback — GET /api/v1/admin/channels/oauth/linkedin/callback
//    PUBLIC endpoint.
// ---------------------------------------------------------------------------

func (h *Handler) LinkedInChannelCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.oauthRedirectError(w, r, "missing_params")
		return
	}

	channel, err := h.services.ChannelOAuth.HandleLinkedInCallback(r.Context(), code, state)
	if err != nil {
		h.logger.Error("linkedin oauth callback failed", "error", err)
		h.oauthRedirectError(w, r, "oauth_failed")
		return
	}

	http.Redirect(w, r, h.oauthRedirectSuccess(channel.ID, channel.ChannelType), http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// 7. DisconnectChannel — POST /api/v1/admin/channels/{id}/disconnect
//    Clears stored OAuth tokens and sets connection_status to 'disconnected'.
// ---------------------------------------------------------------------------

func (h *Handler) DisconnectChannel(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	if err := h.services.ChannelOAuth.DisconnectChannel(r.Context(), channelID); err != nil {
		h.logger.Error("disconnect channel failed", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to disconnect channel")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// ---------------------------------------------------------------------------
// 8. GetChannelStatus — GET /api/v1/admin/channels/{id}/status
//    Returns the OAuth connection status, token expiry, and provider for a
//    channel.
// ---------------------------------------------------------------------------

func (h *Handler) GetChannelStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	status, err := h.services.ChannelOAuth.GetChannelStatus(r.Context(), channelID)
	if err != nil {
		h.logger.Error("get channel status failed", "channel_id", channelID, "error", err)
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ---------------------------------------------------------------------------
// 9. RefreshChannelToken — POST /api/v1/admin/channels/{id}/refresh-token
//    Looks up the channel's oauth_provider and dispatches to the correct
//    provider-specific token refresh method.
// ---------------------------------------------------------------------------

func (h *Handler) RefreshChannelToken(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	if err := h.services.ChannelOAuth.RefreshChannelToken(r.Context(), channelID); err != nil {
		h.logger.Error("refresh channel token failed", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to refresh token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

// ---------------------------------------------------------------------------
// 10. TestChannelConnection — POST /api/v1/admin/channels/{id}/test-connection
//     Makes a lightweight API call to the platform to verify stored tokens
//     are still valid (e.g., YouTube channels.list, Twitter users/me).
//     Always returns HTTP 200 -- the "connected" field indicates success.
// ---------------------------------------------------------------------------

func (h *Handler) TestChannelConnection(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	result, err := h.services.ChannelOAuth.TestConnection(r.Context(), channelID)
	if err != nil {
		h.logger.Error("test channel connection failed", "channel_id", channelID, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// 11. GetQuotaStatus — GET /api/v1/admin/quota?platform=youtube
//     Returns the daily quota usage and limits for a platform. Backed by the
//     Redis-based QuotaTracker.
// ---------------------------------------------------------------------------

func (h *Handler) GetQuotaStatus(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		writeError(w, http.StatusBadRequest, "platform query parameter is required")
		return
	}

	// Allow a per-request limit override via query param (admin tuning).
	limitOverride := 0
	if lo := r.URL.Query().Get("limit_override"); lo != "" {
		if parsed, parseErr := strconv.Atoi(lo); parseErr == nil {
			limitOverride = parsed
		}
	}

	status, err := h.services.ChannelOAuth.GetQuotaStatus(r.Context(), platform, limitOverride)
	if err != nil {
		h.logger.Error("get quota status failed", "platform", platform, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get quota status")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ---------------------------------------------------------------------------
// 12. PublishToPlatform — POST /api/v1/videos/{id}/publish-to-platform
//     Validates the request, inserts a publish_record + upload_queue entry,
//     and returns the record. The background worker picks up the queue item.
// ---------------------------------------------------------------------------

// publishToPlatformRequest is the JSON body for initiating a platform publish.
type publishToPlatformRequest struct {
	Platform     string    `json:"platform"`
	ChannelID    uuid.UUID `json:"channel_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Tags         []string  `json:"tags,omitempty"`
	Visibility   string    `json:"visibility"`
	ScheduledAt  *string   `json:"scheduled_at,omitempty"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
}

func (h *Handler) PublishToPlatform(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	videoID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid video id")
		return
	}

	var req publishToPlatformRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Platform == "" {
		writeError(w, http.StatusBadRequest, "platform is required")
		return
	}
	if req.ChannelID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	// Parse optional scheduled_at.
	var scheduledAt *time.Time
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		t, parseErr := time.Parse(time.RFC3339, *req.ScheduledAt)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "scheduled_at must be RFC3339 format")
			return
		}
		if t.Before(time.Now()) {
			writeError(w, http.StatusBadRequest, "scheduled_at must be in the future")
			return
		}
		scheduledAt = &t
	}

	record, err := h.services.ChannelOAuth.QueuePublish(r.Context(), videoID, service.QueuePublishParams{
		Platform:     req.Platform,
		ChannelID:    req.ChannelID,
		Title:        req.Title,
		Description:  req.Description,
		Tags:         req.Tags,
		Visibility:   req.Visibility,
		ScheduledAt:  scheduledAt,
		ThumbnailURL: req.ThumbnailURL,
	})
	if err != nil {
		h.logger.Error("publish to platform failed",
			"video_id", videoID,
			"platform", req.Platform,
			"channel_id", req.ChannelID,
			"error", err,
		)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, record)
}

// ---------------------------------------------------------------------------
// 13. GetUploadStatus — GET /api/v1/publish-records/{id}/upload-status
//     Returns the current upload/processing status for a publish record.
// ---------------------------------------------------------------------------

func (h *Handler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	recordID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid publish record id")
		return
	}

	status, err := h.services.ChannelOAuth.GetUploadStatus(r.Context(), recordID)
	if err != nil {
		h.logger.Error("get upload status failed", "record_id", recordID, "error", err)
		writeError(w, http.StatusNotFound, "publish record not found")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// ---------------------------------------------------------------------------
// 14. StreamUploadStatus — GET /api/v1/publish-records/{id}/upload-status/stream
//     Server-Sent Events endpoint. Polls the DB every 3 seconds and pushes
//     status updates to the client until the upload reaches a terminal state
//     (completed/published/failed) or the client disconnects.
// ---------------------------------------------------------------------------

func (h *Handler) StreamUploadStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	recordID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid publish record id")
		return
	}

	// Validate that the response writer supports flushing (required for SSE).
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx proxy buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Send an initial status immediately before entering the poll loop.
	if h.sendUploadStatusEvent(w, flusher, ctx, recordID) {
		return // Terminal state reached on first check.
	}

	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case <-ticker.C:
			if h.sendUploadStatusEvent(w, flusher, ctx, recordID) {
				return
			}
		}
	}
}

// sendUploadStatusEvent writes a single SSE data frame with the current upload
// status. Returns true if the stream should close (terminal state or error).
func (h *Handler) sendUploadStatusEvent(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, recordID uuid.UUID) bool {
	status, err := h.services.ChannelOAuth.GetUploadStatus(ctx, recordID)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\":\"record not found\"}\n\n")
		flusher.Flush()
		return true
	}

	data, marshalErr := json.Marshal(status)
	if marshalErr != nil {
		h.logger.Error("marshal upload status for SSE failed", "record_id", recordID, "error", marshalErr)
		return true
	}

	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Close the stream on terminal states so the client knows the upload is done.
	if isTerminalUploadStatus(status.Status) {
		fmt.Fprintf(w, "event: done\ndata: {}\n\n")
		flusher.Flush()
		return true
	}

	return false
}

// isTerminalUploadStatus returns true for statuses that mean the upload
// lifecycle is finished and the SSE stream can be closed.
func isTerminalUploadStatus(status string) bool {
	switch status {
	case "completed", "published", "failed", "cancelled", "removed":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 15. ListPublishHistory — GET /api/v1/admin/publish-history
//     Returns a paginated list of publish records with channel info, filtered
//     by platform, status, channel, and date range.
// ---------------------------------------------------------------------------

func (h *Handler) ListPublishHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	platform := q.Get("platform")
	status := q.Get("status")
	channelIDStr := q.Get("channel_id")
	startDateStr := q.Get("start_date")
	endDateStr := q.Get("end_date")
	limitStr := q.Get("limit")
	offsetStr := q.Get("offset")

	var channelID *uuid.UUID
	if channelIDStr != "" {
		cid, parseErr := uuid.Parse(channelIDStr)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid channel_id")
			return
		}
		channelID = &cid
	}

	var startDate, endDate *time.Time
	if startDateStr != "" {
		t, parseErr := time.Parse("2006-01-02", startDateStr)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "start_date must be YYYY-MM-DD format")
			return
		}
		startDate = &t
	}
	if endDateStr != "" {
		t, parseErr := time.Parse("2006-01-02", endDateStr)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "end_date must be YYYY-MM-DD format")
			return
		}
		// Include the entire end day.
		endOfDay := t.Add(24*time.Hour - time.Nanosecond)
		endDate = &endOfDay
	}

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	records, total, err := h.services.ChannelOAuth.ListPublishHistory(r.Context(), service.PublishHistoryFilter{
		Platform:  platform,
		Status:    status,
		ChannelID: channelID,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list publish history failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list publish history")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  records,
		"total": total,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// oauthRedirectSuccess returns the correct frontend URL based on channel type.
func (h *Handler) oauthRedirectSuccess(channelID uuid.UUID, channelType string) string {
	if channelType == "creator" {
		return fmt.Sprintf("%s/settings/channels?connected=%s", h.cfg.App.FrontendURL, channelID.String())
	}
	return fmt.Sprintf("%s/admin/channels?connected=%s", h.cfg.App.FrontendURL, channelID.String())
}

// oauthRedirectError redirects the user's browser to the frontend channels
// page with an error query parameter. Used by all public OAuth callback
// handlers when something goes wrong. It attempts to look up the channel
// to route creator vs admin errors correctly.
func (h *Handler) oauthRedirectError(w http.ResponseWriter, r *http.Request, errMsg string) {
	// Try to determine channel type from state param to redirect correctly.
	page := "/admin/channels"
	if state := r.URL.Query().Get("state"); state != "" {
		// State format includes channelID — try to parse and look up.
		parts := strings.SplitN(state, ":", 2)
		if len(parts) > 0 {
			if channelID, parseErr := uuid.Parse(parts[0]); parseErr == nil {
				if ch, lookupErr := h.services.Channel.GetByID(r.Context(), channelID); lookupErr == nil && ch.ChannelType == "creator" {
					page = "/settings/channels"
				}
			}
		}
	}
	redirectURL := fmt.Sprintf("%s%s?error=%s", h.cfg.App.FrontendURL, page, errMsg)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// RetryThumbnail handles POST /api/v1/admin/publish-records/{id}/retry-thumbnail.
func (h *Handler) RetryThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := h.requirePathUUID(w, r, "id", "publish record id")
	if !ok {
		return
	}

	if err := h.services.Orchestrator.RetryYouTubeThumbnail(r.Context(), id); err != nil {
		h.logger.Error("retry thumbnail failed", "publish_record_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "thumbnail set"})
}
