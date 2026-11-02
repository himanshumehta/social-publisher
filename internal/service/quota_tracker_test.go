package service

import (
	"encoding/json"
	"testing"
)

func TestDefaultQuotaLimits_AllPlatformsPresent(t *testing.T) {
	expected := map[string]int{
		"youtube":   10000,
		"instagram": 25,
		"threads":   250,
		"twitter":   10000,
		"facebook":  0,
		"linkedin":  100,
	}

	for platform, wantLimit := range expected {
		got, ok := DefaultQuotaLimits[platform]
		if !ok {
			t.Errorf("DefaultQuotaLimits missing platform %q", platform)
			continue
		}
		if got != wantLimit {
			t.Errorf("DefaultQuotaLimits[%q] = %d, want %d", platform, got, wantLimit)
		}
	}

	if len(DefaultQuotaLimits) != len(expected) {
		t.Errorf("DefaultQuotaLimits has %d entries, want %d", len(DefaultQuotaLimits), len(expected))
	}
}

func TestQuotaCosts_AllPlatformsPresent(t *testing.T) {
	expected := map[string]int{
		"youtube":   1600,
		"instagram": 1,
		"threads":   1,
		"twitter":   1,
		"facebook":  0,
		"linkedin":  1,
	}

	for platform, wantCost := range expected {
		got, ok := QuotaCosts[platform]
		if !ok {
			t.Errorf("QuotaCosts missing platform %q", platform)
			continue
		}
		if got != wantCost {
			t.Errorf("QuotaCosts[%q] = %d, want %d", platform, got, wantCost)
		}
	}

	if len(QuotaCosts) != len(expected) {
		t.Errorf("QuotaCosts has %d entries, want %d", len(QuotaCosts), len(expected))
	}
}

func TestDefaultQuotaLimits_NoCriticalZero(t *testing.T) {
	// All platforms except facebook should have limits > 0.
	nonZeroPlatforms := []string{"youtube", "instagram", "threads", "twitter", "linkedin"}

	for _, platform := range nonZeroPlatforms {
		limit, ok := DefaultQuotaLimits[platform]
		if !ok {
			t.Errorf("DefaultQuotaLimits missing platform %q", platform)
			continue
		}
		if limit <= 0 {
			t.Errorf("DefaultQuotaLimits[%q] = %d, expected > 0 for non-facebook platform", platform, limit)
		}
	}
}

func TestQuotaCosts_YouTubeCost(t *testing.T) {
	got, ok := QuotaCosts["youtube"]
	if !ok {
		t.Fatal("QuotaCosts missing youtube entry")
	}
	if got != 1600 {
		t.Errorf("QuotaCosts[youtube] = %d, want 1600 (units per videos.insert)", got)
	}
}

func TestQuotaStatus_FieldNames(t *testing.T) {
	qs := QuotaStatus{
		Platform:     "youtube",
		Used:         3200,
		Limit:        10000,
		ResetsAt:     "2026-02-28T00:00:00Z",
		Period:       "daily",
		DisplayLabel: "2/6 uploads used today",
	}

	data, err := json.Marshal(qs)
	if err != nil {
		t.Fatalf("failed to marshal QuotaStatus: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal QuotaStatus JSON: %v", err)
	}

	expectedTags := []string{"platform", "used", "limit", "resets_at", "period", "display_label"}
	for _, tag := range expectedTags {
		if _, ok := raw[tag]; !ok {
			t.Errorf("QuotaStatus JSON missing expected key %q; got keys: %v", tag, raw)
		}
	}

	if len(raw) != len(expectedTags) {
		t.Errorf("QuotaStatus JSON has %d keys, want %d; got: %v", len(raw), len(expectedTags), raw)
	}
}
