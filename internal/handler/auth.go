package handler

import (
	"net/http"
	"time"

	"github.com/himanshumehta/social-publisher/internal/service"
)

func (h *Handler) GetOAuthURL(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	url, err := h.services.Auth.GetOAuthURL(provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, user, err := h.services.Auth.HandleCallback(r.Context(), provider, body.Code, body.State)
	if err != nil {
		h.logger.Error("oauth callback failed", "provider", provider, "error", err)
		writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	// Set refresh token as httpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(30 * 24 * time.Hour / time.Second),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": tokens.AccessToken,
		"expires_at":   tokens.ExpiresAt,
		"user":         user,
	})
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	tokens, err := h.services.Auth.RefreshAccessToken(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Set new refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(30 * 24 * time.Hour / time.Second),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": tokens.AccessToken,
		"expires_at":   tokens.ExpiresAt,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.services.Auth.Logout(r.Context(), userID); err != nil {
		h.logger.Error("logout failed", "error", err)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	// Signup is temporarily disabled — invite-only membership
	writeError(w, http.StatusForbidden, "signups are currently disabled, ask an administrator for an account")
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, user, err := h.services.Auth.Login(r.Context(), req)
	if err != nil {
		h.logger.Error("login failed", "error", err)
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(30 * 24 * time.Hour / time.Second),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": tokens.AccessToken,
		"expires_at":   tokens.ExpiresAt,
		"user":         user,
	})
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.services.User.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if user.AvatarURL != "" {
		if presigned, err := h.services.User.PresignAvatarURL(r.Context(), user.AvatarURL); err == nil && presigned != "" {
			user.AvatarURL = presigned
		}
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.services.Auth.ForgotPassword(r.Context(), body.Email); err != nil {
		h.logger.Error("forgot password error", "error", err)
	}

	// Always return success to prevent email enumeration
	writeJSON(w, http.StatusOK, map[string]string{"message": "If that email exists, a reset link has been sent"})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.services.Auth.ResetPassword(r.Context(), body.Token, body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Password has been reset"})
}
