package handler

import (
	"net/http"

	"github.com/himanshumehta/social-publisher/internal/service"
)

const refreshCookieName = "refresh_token"

// setRefreshCookie stores the refresh token in an HttpOnly cookie scoped to
// the auth endpoints. maxAge < 0 deletes the cookie.
func (h *Handler) setRefreshCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (h *Handler) refreshCookieMaxAge() int {
	return int(h.cfg.JWT.RefreshTokenTTL.Seconds())
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if !h.requireJSON(w, r, &req) {
		return
	}

	tokens, user, err := h.services.Auth.Login(r.Context(), req)
	if err != nil {
		h.logger.Warn("login failed", "error", err)
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	h.setRefreshCookie(w, tokens.RefreshToken, h.refreshCookieMaxAge())
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": tokens.AccessToken,
		"expires_at":   tokens.ExpiresAt,
		"user":         user,
	})
}

// RefreshToken handles POST /api/v1/auth/refresh. The refresh token is
// rotated on every call; reuse of an old token revokes the whole family.
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	tokens, err := h.services.Auth.RefreshAccessToken(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	h.setRefreshCookie(w, tokens.RefreshToken, h.refreshCookieMaxAge())
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": tokens.AccessToken,
		"expires_at":   tokens.ExpiresAt,
	})
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	if err := h.services.Auth.Logout(r.Context(), userID); err != nil {
		h.logger.Error("logout failed", "error", err)
	}

	h.setRefreshCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// GetMe handles GET /api/v1/auth/me.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUserID(w, r)
	if !ok {
		return
	}

	user, err := h.services.User.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, user)
}
