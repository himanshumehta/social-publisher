package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// APIKeyContext holds the validated API key metadata extracted from the request.
type APIKeyContext struct {
	KeyID  uuid.UUID
	UserID uuid.UUID
	Scopes []string
}

// HasScope checks whether this API key has a specific permission scope.
func (c *APIKeyContext) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

type apiKeyContextKey struct{}

// GetAPIKey retrieves the validated API key context from the request.
// Returns nil if the request was not authenticated via API key.
func GetAPIKey(r *http.Request) *APIKeyContext {
	ctx, _ := r.Context().Value(apiKeyContextKey{}).(*APIKeyContext)
	return ctx
}

// RequireAPIKey returns middleware that validates Bearer token API keys.
// It hashes the raw key with SHA-256 and looks it up via the APIKeyService.
func RequireAPIKey(apiKeySvc *service.APIKeyService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"missing API key"}`, http.StatusUnauthorized)
				return
			}

			rawKey := strings.TrimPrefix(auth, "Bearer ")
			if apiKeySvc == nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			apiKey, err := apiKeySvc.ValidateKey(r.Context(), service.HashAPIKey(rawKey))
			if err != nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey{}, &APIKeyContext{
				KeyID:  apiKey.ID,
				UserID: apiKey.UserID,
				Scopes: apiKey.Scopes,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
