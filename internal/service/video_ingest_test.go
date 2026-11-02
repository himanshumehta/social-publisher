package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestNormalizeIngestMetadata_DefaultsAndDedupes(t *testing.T) {
	m := &model.VideoIngestMetadata{
		Title:           "  Classic 350: five-year cost  ",
		Description:     "What it costs to own, year by year.",
		TargetPlatforms: []string{"Instagram", "youtube", "instagram"},
		PlatformPosts:   map[string]*model.IngestPlatformText{"youtube": {Text: " Long description "}},
	}
	if err := NormalizeIngestMetadata(m); err != nil {
		t.Fatalf("NormalizeIngestMetadata: %v", err)
	}
	if m.Title != "Classic 350: five-year cost" {
		t.Errorf("title not trimmed: %q", m.Title)
	}
	if got := strings.Join(m.TargetPlatforms, ","); got != "instagram,youtube" {
		t.Errorf("platforms = %s, want instagram,youtube", got)
	}
	if m.PlatformPosts["instagram"].Text != m.Description {
		t.Errorf("instagram caption = %q, want the description", m.PlatformPosts["instagram"].Text)
	}
	if m.PlatformPosts["youtube"].Text != "Long description" {
		t.Errorf("youtube caption = %q", m.PlatformPosts["youtube"].Text)
	}
}

func TestNormalizeIngestMetadata_Rejects(t *testing.T) {
	long := strings.Repeat("x", model.VideoCaptionLimits["threads"]+1)
	cases := map[string]model.VideoIngestMetadata{
		"no title":             {TargetPlatforms: []string{"youtube"}},
		"no platforms":         {Title: "t"},
		"unknown platform":     {Title: "t", TargetPlatforms: []string{"myspace"}},
		"caption over limit":   {Title: "t", Description: long, TargetPlatforms: []string{"threads"}},
		"caption not targeted": {Title: "t", TargetPlatforms: []string{"youtube"}, PlatformPosts: map[string]*model.IngestPlatformText{"facebook": {Text: "x"}}},
		"empty caption":        {Title: "t", TargetPlatforms: []string{"youtube"}, PlatformPosts: map[string]*model.IngestPlatformText{"youtube": {Text: "  "}}},
	}
	for name, m := range cases {
		err := NormalizeIngestMetadata(&m)
		if !errors.Is(err, ErrInvalidIngest) {
			t.Errorf("%s: err = %v, want ErrInvalidIngest", name, err)
		}
	}
}

func TestIngestRequestHash_CoversEveryInput(t *testing.T) {
	base := model.VideoIngestMetadata{Title: "t", TargetPlatforms: []string{"youtube"}}
	if err := NormalizeIngestMetadata(&base); err != nil {
		t.Fatal(err)
	}
	h0, _ := IngestRequestHash(&base, "v", "")
	if again, _ := IngestRequestHash(&base, "v", ""); again != h0 {
		t.Fatal("hash is not deterministic")
	}
	other := base
	other.Title = "u"
	variants := map[string]string{}
	variants["metadata"], _ = IngestRequestHash(&other, "v", "")
	variants["video"], _ = IngestRequestHash(&base, "w", "")
	variants["thumbnail"], _ = IngestRequestHash(&base, "v", "t")
	for name, h := range variants {
		if h == h0 {
			t.Errorf("changing the %s did not change the hash", name)
		}
	}
}
