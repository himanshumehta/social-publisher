package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func userIDFromRequest(r *http.Request) (uuid.UUID, bool) {
	return middleware.UserIDFromContext(r.Context())
}

func (h *Handler) Onboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.OnboardRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}

	if err := h.services.User.Onboard(r.Context(), userID, req); err != nil {
		h.logger.Error("onboard failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "onboarding failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "onboarding complete"})
}

func (h *Handler) SubmitBanking(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.BankingRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if req.UPIID == "" && (req.AccountNumber == "" || req.IFSCCode == "") {
		writeError(w, http.StatusBadRequest, "provide UPI ID or account number + IFSC code")
		return
	}

	if err := h.services.User.SubmitBanking(r.Context(), userID, req); err != nil {
		h.logger.Error("submit banking failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "failed to submit banking details")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "banking details submitted"})
}

func (h *Handler) GetAvatarUploadURL(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	contentType := r.URL.Query().Get("content_type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ext := "jpg"
	switch contentType {
	case "image/png":
		ext = "png"
	case "image/webp":
		ext = "webp"
	}

	key := fmt.Sprintf("avatars/%s.%s", userID.String(), ext)
	url, err := h.services.Upload.PresignAvatarUpload(r.Context(), key, contentType, 15*time.Minute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate upload URL")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"upload_url":  url,
		"storage_key": key,
	})
}

func (h *Handler) ConfirmAvatarUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		StorageKey string `json:"storage_key"`
	}
	if err := readJSON(r, &req); err != nil || req.StorageKey == "" {
		writeError(w, http.StatusBadRequest, "storage_key is required")
		return
	}

	if err := h.services.User.UpdateAvatarURL(r.Context(), userID, req.StorageKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update avatar")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "avatar updated"})
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	var req service.UpdateProfileRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}

	user, err := h.services.User.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		h.logger.Error("update profile failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	if user.AvatarURL != "" {
		if presigned, err := h.services.User.PresignAvatarURL(r.Context(), user.AvatarURL); err == nil && presigned != "" {
			user.AvatarURL = presigned
		}
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	profile, err := h.services.User.GetCreatorProfile(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) GetBankingInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	info, err := h.services.User.GetBankingInfo(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get banking info")
		return
	}

	if info == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) GetOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	status, err := h.services.User.GetOnboardingStatus(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get onboarding status")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.services.Category.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list categories")
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) SaveOnboardDraft(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	// Read the request body as raw JSON
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Get the raw JSON from body
	var draft map[string]interface{}
	if !h.requireJSON(w, r, &draft) {
		return
	}

	// Convert back to raw JSON
	rawJSON, err := json.Marshal(draft)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.services.User.SaveOnboardDraft(r.Context(), userID, rawJSON); err != nil {
		h.logger.Error("save onboard draft failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "failed to save draft")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "draft saved"})
}

func (h *Handler) GetCreatorCountByCity(w http.ResponseWriter, r *http.Request) {
	city := r.URL.Query().Get("city")
	if city == "" {
		writeError(w, http.StatusBadRequest, "city parameter is required")
		return
	}

	count, err := h.services.User.GetCreatorCountByCity(r.Context(), city)
	if err != nil {
		h.logger.Error("get creator count failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get creator count")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}
