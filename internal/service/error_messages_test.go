package service

import "testing"

const genericFallback = "An error occurred during publishing. Please try again or contact support."

func TestMapPlatformError_ExactMatch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Google/YouTube
		{
			name:     "invalid_grant exact match",
			input:    "invalid_grant",
			expected: "YouTube connection expired. Please reconnect your channel in Settings \u2192 Channels.",
		},
		{
			name:     "quotaExceeded exact match",
			input:    "quotaExceeded",
			expected: "Daily YouTube limit reached. Your upload is queued for tomorrow at 12:01 AM UTC.",
		},
		// Meta
		{
			name:     "CONTAINER_EXPIRED exact match",
			input:    "CONTAINER_EXPIRED",
			expected: "Upload timed out. Retrying automatically...",
		},
		{
			name:     "OAuthException exact match",
			input:    "OAuthException",
			expected: "Platform connection lost. Please reconnect your account.",
		},
		// Twitter
		{
			name:     "refresh_token_invalid exact match",
			input:    "refresh_token_invalid",
			expected: "Twitter connection lost. Please reconnect your account.",
		},
		{
			name:     "DUPLICATE_CONTENT exact match",
			input:    "DUPLICATE_CONTENT",
			expected: "This content was already posted to Twitter.",
		},
		// LinkedIn
		{
			name:     "UPLOAD_FAILED exact match",
			input:    "UPLOAD_FAILED",
			expected: "LinkedIn upload failed. Try again with a file under 200MB.",
		},
		{
			name:     "INSUFFICIENT_PERMISSIONS exact match",
			input:    "INSUFFICIENT_PERMISSIONS",
			expected: "Missing permissions. Please reconnect your account.",
		},
		// Generic
		{
			name:     "NETWORK_TIMEOUT exact match",
			input:    "NETWORK_TIMEOUT",
			expected: "Upload interrupted. Resuming automatically...",
		},
		{
			name:     "FILE_TOO_LARGE exact match",
			input:    "FILE_TOO_LARGE",
			expected: "Video exceeds the platform's size limit. Try compressing the file.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapPlatformError(tt.input)
			if got != tt.expected {
				t.Errorf("MapPlatformError(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMapPlatformError_PartialMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "error message containing quotaExceeded",
			input: "Error 403: quotaExceeded - daily limit",
		},
		{
			name:  "error message containing invalid_grant",
			input: "oauth2: token refresh failed: invalid_grant",
		},
		{
			name:  "error message containing OAuthException",
			input: "GraphAPI error: OAuthException code 190",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapPlatformError(tt.input)
			if got == genericFallback {
				t.Errorf("MapPlatformError(%q) returned generic fallback, expected a specific match", tt.input)
			}
		})
	}
}

func TestMapPlatformError_CaseInsensitiveMatch(t *testing.T) {
	// The function lowercases both the input and the key for partial matching,
	// so mixed-case input should still match.
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "lowercase quotaexceeded in error message",
			input: "error: quotaexceeded happened",
		},
		{
			name:  "mixed case OauthException in error message",
			input: "error: oauthexception occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapPlatformError(tt.input)
			if got == genericFallback {
				t.Errorf("MapPlatformError(%q) returned generic fallback, expected a case-insensitive match", tt.input)
			}
		})
	}
}

func TestMapPlatformError_UnknownError(t *testing.T) {
	got := MapPlatformError("some_completely_unknown_error_xyz")
	if got != genericFallback {
		t.Errorf("MapPlatformError(unknown) = %q, want generic fallback %q", got, genericFallback)
	}
}

func TestMapPlatformError_EmptyString(t *testing.T) {
	got := MapPlatformError("")
	if got != genericFallback {
		t.Errorf("MapPlatformError(\"\") = %q, want generic fallback %q", got, genericFallback)
	}
}

func TestMapPlatformError_AllPlatformCategories(t *testing.T) {
	// Verify at least one key from each platform category returns a non-fallback message.
	categories := map[string]string{
		"Google":   "PROCESSING_FAILED",
		"Meta":     "RATE_LIMIT_EXCEEDED",
		"Twitter":  "MEDIA_PROCESSING_FAILED",
		"LinkedIn": "INVALID_ORGANIZATION",
		"Generic":  "S3_DOWNLOAD_FAILED",
	}

	for category, key := range categories {
		t.Run(category, func(t *testing.T) {
			got := MapPlatformError(key)
			if got == genericFallback {
				t.Errorf("MapPlatformError(%q) for %s category returned generic fallback", key, category)
			}
		})
	}
}
