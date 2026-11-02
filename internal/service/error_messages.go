package service

import "strings"

// PlatformErrorMap maps technical platform errors to admin-friendly messages.
var PlatformErrorMap = map[string]string{
	// Google/YouTube
	"invalid_grant":       "YouTube connection expired. Please reconnect your channel in Settings → Channels.",
	"quotaExceeded":       "Daily YouTube limit reached. Your upload is queued for tomorrow at 12:01 AM UTC.",
	"PROCESSING_FAILED":   "YouTube couldn't process this video. Try re-uploading.",
	"uploadLimitExceeded": "YouTube upload limit reached. Try again later.",
	"videoNotFound":       "YouTube video not found. It may have been deleted from YouTube.",
	"forbidden":           "YouTube access denied. Please reconnect your channel.",

	// Meta (Instagram/Facebook/Threads)
	"CONTAINER_EXPIRED":        "Upload timed out. Retrying automatically...",
	"CONTAINER_ERROR":          "Platform couldn't process the video. Check format and try again.",
	"RATE_LIMIT_EXCEEDED":      "Too many requests. Retrying shortly...",
	"PUBLISHING_LIMIT_REACHED": "Daily posting limit reached. Upload is queued for later.",
	"INVALID_VIDEO_FORMAT":     "This video format isn't supported. Check resolution and duration requirements.",
	"OAuthException":           "Platform connection lost. Please reconnect your account.",
	"MEDIA_TYPE_NOT_SUPPORTED": "This media type isn't supported by the platform.",

	// Twitter/X
	"refresh_token_invalid":   "Twitter connection lost. Please reconnect your account.",
	"MEDIA_PROCESSING_FAILED": "Twitter couldn't process this video. Try a shorter clip (max 140 seconds).",
	"DUPLICATE_CONTENT":       "This content was already posted to Twitter.",
	"TWEET_CREATION_FAILED":   "Failed to create tweet. Please try again.",

	// LinkedIn
	"UPLOAD_FAILED":            "LinkedIn upload failed. Try again with a file under 200MB.",
	"INVALID_ORGANIZATION":     "LinkedIn company page not found. Check your connection.",
	"INSUFFICIENT_PERMISSIONS": "Missing permissions. Please reconnect your account.",

	// Generic
	"NETWORK_TIMEOUT":      "Upload interrupted. Resuming automatically...",
	"FILE_TOO_LARGE":       "Video exceeds the platform's size limit. Try compressing the file.",
	"S3_DOWNLOAD_FAILED":   "Failed to retrieve video file. Retrying...",
	"TOKEN_REFRESH_FAILED": "Platform connection expired. Please reconnect your channel.",
	"UNSUPPORTED_PLATFORM": "This platform is not yet supported for API publishing.",
}

// MapPlatformError returns an admin-friendly message for a technical error.
// Falls back to a generic message if no mapping exists.
func MapPlatformError(technicalError string) string {
	// Check exact match first
	if msg, ok := PlatformErrorMap[technicalError]; ok {
		return msg
	}

	// Check if the error contains any known patterns
	lower := strings.ToLower(technicalError)
	for key, msg := range PlatformErrorMap {
		if strings.Contains(lower, strings.ToLower(key)) {
			return msg
		}
	}

	// Generic fallback
	return "An error occurred during publishing. Please try again or contact support."
}
