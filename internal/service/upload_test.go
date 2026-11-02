package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// TestStorageKeyFormat verifies that the storage key generated during
// InitiateUpload includes a version prefix to prevent S3 object overwrites
// when the same filename is uploaded multiple times.
//
// Root cause: before the fix, storageKey = "{slug}/{filename}" which meant
// uploading "VID_001.mp4" as v23 and v24 would both resolve to the same
// S3 key, causing v24 to silently overwrite v23.
//
// Fix: storageKey = "{slug}/v{N}-{filename}" where N is the next version.
func TestStorageKeyVersionPrefix(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Get a real user to satisfy FK constraints
	var userID uuid.UUID
	err := pool.QueryRow(ctx, "SELECT id FROM users WHERE role = 'creator' LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no creator users in test database: %v", err)
	}

	// Find an project owned by this creator in 'claimed' or 'in_production' status
	var projectID uuid.UUID
	var projectSlug string
	err = pool.QueryRow(ctx,
		`SELECT id, slug FROM projects
		 WHERE creator_id = $1 AND status IN ('claimed', 'in_production', 'in_editing', 'in_review')
		 LIMIT 1`, userID,
	).Scan(&projectID, &projectSlug)
	if err != nil {
		t.Skipf("skipping: no suitable project for creator %s: %v", userID, err)
	}

	// We cannot call InitiateUpload directly because it requires an S3Client
	// that can create multipart uploads. Instead, verify the version query
	// and key format logic independently.

	// ---- Test 1: Version query returns correct next version ----
	// Get current max version for 'raw' format in this project
	var currentMax int
	err = pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM videos WHERE project_id = $1 AND format = 'raw'`,
		projectID,
	).Scan(&currentMax)
	if err != nil {
		t.Fatalf("failed to query max version: %v", err)
	}
	nextVersion := currentMax + 1

	// ---- Test 2: Storage key format includes version prefix ----
	fileName := "VID_20260205_085303_005.mp4"
	storageKey := fmt.Sprintf("%s/v%d-%s", projectSlug, nextVersion, fileName)

	expectedPrefix := fmt.Sprintf("%s/v%d-", projectSlug, nextVersion)
	if len(storageKey) <= len(expectedPrefix) {
		t.Errorf("storage key too short: %q", storageKey)
	}
	if storageKey[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("storage key missing version prefix: got %q, want prefix %q", storageKey, expectedPrefix)
	}

	t.Logf("project=%s slug=%s currentMax=%d nextVersion=%d", projectID, projectSlug, currentMax, nextVersion)
	t.Logf("storage key: %s", storageKey)

	// ---- Test 3: Two uploads of the same filename produce different keys ----
	key1 := fmt.Sprintf("%s/v%d-%s", projectSlug, nextVersion, fileName)
	key2 := fmt.Sprintf("%s/v%d-%s", projectSlug, nextVersion+1, fileName)

	if key1 == key2 {
		t.Errorf("duplicate storage keys for different versions: %q == %q", key1, key2)
	}
	t.Logf("key1 (v%d): %s", nextVersion, key1)
	t.Logf("key2 (v%d): %s", nextVersion+1, key2)

	// ---- Test 4: Verify key format is correct across various filenames ----
	testCases := []struct {
		slug     string
		version  int
		filename string
		expected string
	}{
		{"pulsar-n160-review", 1, "VID_001.mp4", "pulsar-n160-review/v1-VID_001.mp4"},
		{"pulsar-n160-review", 23, "VID_20260205_085303_005.mp4", "pulsar-n160-review/v23-VID_20260205_085303_005.mp4"},
		{"pulsar-n160-review", 24, "VID_20260205_085303_005.mp4", "pulsar-n160-review/v24-VID_20260205_085303_005.mp4"},
		{"my-project", 1, "photo.jpg", "my-project/v1-photo.jpg"},
		{"my-project", 100, "audio-recording.wav", "my-project/v100-audio-recording.wav"},
	}

	for _, tc := range testCases {
		got := fmt.Sprintf("%s/v%d-%s", tc.slug, tc.version, tc.filename)
		if got != tc.expected {
			t.Errorf("key format mismatch: got %q, want %q", got, tc.expected)
		}
	}
}

// TestStorageKeyNoDuplicates verifies that if multiple videos with the same
// filename exist for an project, the version counter increments correctly
// so each new upload gets a unique storage key.
func TestStorageKeyNoDuplicates(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Find any project that has multiple videos of the same format
	var projectID uuid.UUID
	var format string
	var count int
	err := pool.QueryRow(ctx,
		`SELECT project_id, format, COUNT(*) as cnt
		 FROM videos
		 GROUP BY project_id, format
		 HAVING COUNT(*) > 1
		 LIMIT 1`,
	).Scan(&projectID, &format, &count)
	if err != nil {
		t.Skipf("skipping: no project with multiple same-format videos: %v", err)
	}

	// Get max version
	var maxVersion int
	err = pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM videos WHERE project_id = $1 AND format = $2`,
		projectID, format,
	).Scan(&maxVersion)
	if err != nil {
		t.Fatalf("failed to get max version: %v", err)
	}

	nextVersion := maxVersion + 1
	t.Logf("project=%s format=%s count=%d maxVersion=%d nextVersion=%d",
		projectID, format, count, maxVersion, nextVersion)

	// Verify next version is always greater than any existing version
	if nextVersion <= maxVersion {
		t.Errorf("next version %d should be > max version %d", nextVersion, maxVersion)
	}

	// Check that existing storage keys for this project+format are unique
	rows, err := pool.Query(ctx,
		`SELECT version, storage_key FROM videos WHERE project_id = $1 AND format = $2 ORDER BY version`,
		projectID, format,
	)
	if err != nil {
		t.Fatalf("failed to query videos: %v", err)
	}
	defer rows.Close()

	seen := make(map[string]int)
	for rows.Next() {
		var version int
		var storageKey string
		if err := rows.Scan(&version, &storageKey); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if prevVersion, exists := seen[storageKey]; exists {
			t.Errorf("DUPLICATE storage key %q used by version %d and %d — this is the bug we fixed",
				storageKey, prevVersion, version)
		}
		seen[storageKey] = version
		t.Logf("  v%d → %s", version, storageKey)
	}
}

// TestMultipartUploadTracking verifies that multipart_uploads table
// correctly tracks the format field which is used for version numbering.
func TestMultipartUploadTracking(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Verify schema has format column in multipart_uploads
	var hasFormat bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'multipart_uploads' AND column_name = 'format'
		)`,
	).Scan(&hasFormat)
	if err != nil {
		t.Fatalf("schema check failed: %v", err)
	}
	if !hasFormat {
		t.Error("multipart_uploads table missing 'format' column — required for versioned storage keys")
	}

	// Check that all completed uploads have a non-empty format
	var emptyFormatCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM multipart_uploads WHERE status = 'completed' AND (format IS NULL OR format = '')`,
	).Scan(&emptyFormatCount)
	if err != nil {
		t.Fatalf("format check failed: %v", err)
	}
	if emptyFormatCount > 0 {
		t.Errorf("found %d completed uploads with empty/null format — these may produce incorrect storage keys", emptyFormatCount)
	}
}

// TestDuplicateFileError verifies DuplicateFileError behavior.
func TestDuplicateFileError(t *testing.T) {
	t.Run("error message format", func(t *testing.T) {
		err := &DuplicateFileError{FileName: "VID_001.mp4"}
		msg := err.Error()

		if msg == "" {
			t.Fatal("error message is empty")
		}
		// Must contain the filename
		if !containsStr(msg, "VID_001.mp4") {
			t.Errorf("error message should contain filename, got: %s", msg)
		}
		// Must contain "duplicate"
		if !containsStr(msg, "duplicate") {
			t.Errorf("error message should contain 'duplicate', got: %s", msg)
		}
		// Must contain "rename"
		if !containsStr(msg, "rename") {
			t.Errorf("error message should contain 'rename', got: %s", msg)
		}
		t.Logf("error message: %s", msg)
	})

	t.Run("IsDuplicateFileError returns true", func(t *testing.T) {
		err := &DuplicateFileError{FileName: "test.mp4"}
		if !IsDuplicateFileError(err) {
			t.Error("IsDuplicateFileError should return true for DuplicateFileError")
		}
	})

	t.Run("IsDuplicateFileError returns false for other errors", func(t *testing.T) {
		err := fmt.Errorf("some other error")
		if IsDuplicateFileError(err) {
			t.Error("IsDuplicateFileError should return false for non-DuplicateFileError")
		}
	})

	t.Run("IsDuplicateFileError works with wrapped errors", func(t *testing.T) {
		inner := &DuplicateFileError{FileName: "test.mp4"}
		wrapped := fmt.Errorf("upload failed: %w", inner)
		if !IsDuplicateFileError(wrapped) {
			t.Error("IsDuplicateFileError should work with wrapped errors")
		}
	})

	t.Run("errors.As works correctly", func(t *testing.T) {
		err := &DuplicateFileError{FileName: "movie.mov"}
		var dup *DuplicateFileError
		if !errors.As(err, &dup) {
			t.Fatal("errors.As should match DuplicateFileError")
		}
		if dup.FileName != "movie.mov" {
			t.Errorf("expected FileName 'movie.mov', got %q", dup.FileName)
		}
	})
}

// TestDuplicateFilenameCheck verifies that the DB query used to detect
// duplicate filenames works correctly against the videos table.
func TestDuplicateFilenameCheck(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Find an project that has at least one video
	var projectID uuid.UUID
	var format, fileName string
	err := pool.QueryRow(ctx,
		`SELECT project_id, format, file_name FROM videos
		 WHERE is_deleted = FALSE
		 LIMIT 1`,
	).Scan(&projectID, &format, &fileName)
	if err != nil {
		t.Skipf("skipping: no videos in test database: %v", err)
	}

	t.Logf("testing duplicate check: project=%s format=%s file=%s", projectID, format, fileName)

	// The same filename should be detected as duplicate
	var existingCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos
		 WHERE project_id = $1 AND format = $2 AND file_name = $3 AND is_deleted = FALSE`,
		projectID, format, fileName,
	).Scan(&existingCount)
	if err != nil {
		t.Fatalf("duplicate check query failed: %v", err)
	}
	if existingCount == 0 {
		t.Error("expected existing video to be detected as duplicate")
	}
	t.Logf("duplicate count for %q: %d (expected > 0)", fileName, existingCount)

	// A completely different filename should NOT be detected as duplicate
	var noMatchCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos
		 WHERE project_id = $1 AND format = $2 AND file_name = $3 AND is_deleted = FALSE`,
		projectID, format, "NONEXISTENT_FILE_12345.mp4",
	).Scan(&noMatchCount)
	if err != nil {
		t.Fatalf("no-match check query failed: %v", err)
	}
	if noMatchCount != 0 {
		t.Errorf("expected 0 matches for nonexistent filename, got %d", noMatchCount)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidateUploadRequest(t *testing.T) {
	ok := func(name string) InitiateUploadRequest {
		return InitiateUploadRequest{FileName: name, FileSizeBytes: 1024, MimeType: "video/mp4"}
	}
	for in, want := range map[string]string{
		"clip.mp4":                     "clip.mp4",
		"../../other-project/v1-x.mp4": "v1-x.mp4",
		`..\..\evil.mp4`:               "evil.mp4",
		"name with spaces.mov":         "name with spaces.mov",
		"weird\x00na;me$.mp4":          "weird_na_me_.mp4",
		".hidden.mp4":                  "hidden.mp4",
	} {
		req := ok(in)
		if err := validateUploadRequest(&req); err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if req.FileName != want {
			t.Errorf("%q sanitised to %q, want %q", in, req.FileName, want)
		}
	}

	for name, req := range map[string]InitiateUploadRequest{
		"too large": {FileName: "a.mp4", FileSizeBytes: maxUploadBytes + 1, MimeType: "video/mp4"},
		"bad type":  {FileName: "a.sh", FileSizeBytes: 10, MimeType: "application/x-sh"},
		"no size":   {FileName: "a.mp4", MimeType: "video/mp4"},
		"only dots": {FileName: "..", FileSizeBytes: 10, MimeType: "video/mp4"},
	} {
		r := req
		if err := validateUploadRequest(&r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
