package handler

import (
	"fmt"
	"net/http"
	"strings"
)

func (h *Handler) ListMyChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	channels, err := h.services.Channel.ListByUser(r.Context(), userID)
	if err != nil {
		h.logger.Error("list my channels failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list channels")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": channels})
}

// providerToPlatform maps an OAuth provider name to the channel platform.
func providerToPlatform(provider string) (string, bool) {
	switch strings.ToLower(provider) {
	case "google":
		return "youtube", true
	case "facebook":
		return "facebook", true
	case "instagram":
		return "instagram", true
	case "threads":
		return "threads", true
	case "twitter":
		return "twitter", true
	case "linkedin":
		return "linkedin", true
	default:
		return "", false
	}
}

// ConnectMyChannel initiates OAuth for a creator to connect their own channel.
// POST /api/v1/me/channels/connect
func (h *Handler) ConnectMyChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var body struct {
		Provider string `json:"provider"`
	}
	if !h.requireJSON(w, r, &body) {
		return
	}
	provider := body.Provider
	platform, valid := providerToPlatform(provider)
	if !valid {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported provider: %s", provider))
		return
	}

	// Check if user already has this platform connected.
	exists, err := h.services.Channel.HasPlatformChannel(r.Context(), userID, platform)
	if err != nil {
		h.logger.Error("check platform channel failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check existing channel")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, fmt.Sprintf("you already have a %s channel connected", platform))
		return
	}

	// Create placeholder channel.
	channel, err := h.services.Channel.CreateCreatorChannel(r.Context(), userID, platform, "connecting...", "", "")
	if err != nil {
		h.logger.Error("create placeholder channel failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}

	// Get OAuth URL using the same switch as admin.
	var authURL string
	switch strings.ToLower(provider) {
	case "google":
		authURL, err = h.services.ChannelOAuth.GetGoogleOAuthURL(r.Context(), channel.ID)
	case "facebook":
		authURL, err = h.services.ChannelOAuth.GetMetaOAuthURL(r.Context(), channel.ID)
	case "instagram":
		authURL, err = h.services.ChannelOAuth.GetInstagramBusinessLoginURL(r.Context(), channel.ID)
	case "threads":
		authURL, err = h.services.ChannelOAuth.GetThreadsOAuthURL(r.Context(), channel.ID)
	case "twitter":
		authURL, err = h.services.ChannelOAuth.GetTwitterOAuthURL(r.Context(), channel.ID)
	case "linkedin":
		authURL, err = h.services.ChannelOAuth.GetLinkedInOAuthURL(r.Context(), channel.ID)
	}

	if err != nil {
		h.logger.Error("get oauth url failed", "provider", provider, "channel_id", channel.ID, "error", err)
		// Clean up placeholder on failure.
		_ = h.services.Channel.DeleteByUser(r.Context(), channel.ID, userID)
		writeError(w, http.StatusInternalServerError, "failed to generate oauth url")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"url":        authURL,
		"channel_id": channel.ID.String(),
	})
}

// DisconnectMyChannel disconnects a creator's own channel.
// POST /api/v1/me/channels/{id}/disconnect
func (h *Handler) DisconnectMyChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.requirePathUUID(w, r, "id", "channel id")
	if !ok {
		return
	}

	// Verify ownership.
	if _, err := h.services.Channel.GetByIDAndUser(r.Context(), channelID, userID); err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	if err := h.services.ChannelOAuth.DisconnectChannel(r.Context(), channelID); err != nil {
		h.logger.Error("disconnect my channel failed", "channel_id", channelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to disconnect channel")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// GetMyChannelStatus returns the OAuth status of a creator's own channel.
// GET /api/v1/me/channels/{id}/status
func (h *Handler) GetMyChannelStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.requirePathUUID(w, r, "id", "channel id")
	if !ok {
		return
	}

	// Verify ownership.
	if _, err := h.services.Channel.GetByIDAndUser(r.Context(), channelID, userID); err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	status, err := h.services.ChannelOAuth.GetChannelStatus(r.Context(), channelID)
	if err != nil {
		h.logger.Error("get my channel status failed", "channel_id", channelID, "error", err)
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// DeleteMyChannel removes a creator's own channel.
// DELETE /api/v1/me/channels/{id}
func (h *Handler) DeleteMyChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.requirePathUUID(w, r, "id", "channel id")
	if !ok {
		return
	}

	if err := h.services.Channel.DeleteByUser(r.Context(), channelID, userID); err != nil {
		h.logger.Error("delete my channel failed", "channel_id", channelID, "error", err)
		if strings.Contains(err.Error(), "active publish records") {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusNotFound, "channel not found")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ListAvailableChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	projectID, ok := h.requirePathUUID(w, r, "id", "project id")
	if !ok {
		return
	}

	channels, err := h.services.Channel.ListAvailableForProject(r.Context(), projectID, userID)
	if err != nil {
		h.logger.Error("list available channels failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list available channels")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": channels})
}
