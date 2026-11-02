package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func newBufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestDryRunAdapter_LogsAndReportsSuccess(t *testing.T) {
	logger, buf := newBufferLogger()
	postID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	id, url, err := DryRunAdapter{Logger: logger}.Publish(context.Background(), PublishRequest{
		Platform: "instagram",
		PostID:   postID,
		Text:     "Hello from social-publisher",
	})
	if err != nil {
		t.Fatalf("dry-run publish returned error: %v", err)
	}
	if id != "dryrun-instagram-11111111" {
		t.Errorf("unexpected platform id %q", id)
	}
	if !strings.HasPrefix(url, "https://dry-run.invalid/instagram/") {
		t.Errorf("unexpected platform url %q", url)
	}

	want := "would publish instagram 11111111-2222-3333-4444-555555555555 Hello from social-publisher"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("log output missing %q\ngot: %s", want, buf.String())
	}
}

func TestCaptionPrefix(t *testing.T) {
	long := strings.Repeat("a", 50)
	if got := captionPrefix(long, 40); got != strings.Repeat("a", 40)+"..." {
		t.Errorf("long caption not truncated: %q", got)
	}
	if got := captionPrefix("line one\nline   two", 40); got != "line one line two" {
		t.Errorf("whitespace not collapsed: %q", got)
	}
}

func TestPostPublisher_EnableDryRun_BindsEveryPlatform(t *testing.T) {
	logger, _ := newBufferLogger()
	pp := NewPostPublisher(nil, logger, nil)

	for _, p := range SupportedPostPlatforms {
		if _, ok := pp.adapters[p].(PlatformAdapterFunc); !ok {
			t.Fatalf("expected real adapter for %s before dry-run, got %T", p, pp.adapters[p])
		}
	}

	pp.EnableDryRun()
	if !pp.dryRun {
		t.Fatal("dryRun = false after EnableDryRun")
	}
	for _, p := range SupportedPostPlatforms {
		if _, ok := pp.adapters[p].(DryRunAdapter); !ok {
			t.Errorf("platform %s not routed to DryRunAdapter (got %T)", p, pp.adapters[p])
		}
	}
}

func TestPostPublisher_DryRun_MarksPublishedWithoutCredentials(t *testing.T) {
	logger, buf := newBufferLogger()
	pp := NewPostPublisher(nil, logger, nil)
	pp.EnableDryRun()

	post := &model.Post{ID: uuid.New()}
	for _, platform := range SupportedPostPlatforms {
		buf.Reset()
		pp2 := &model.PlatformPost{Platform: platform, Text: "Launch day", Hashtags: []string{"go"}}
		// Placeholder channel: no ID, no access token.
		ch := &model.Channel{Platform: platform, ChannelName: "dry-run"}

		res := pp.publishToPlatform(context.Background(), post, pp2, ch)

		if res.Status != "published" {
			t.Fatalf("%s: status = %q (error %q), want published", platform, res.Status, res.Error)
		}
		if res.PublishedAt == nil || res.PlatformPostID == "" || res.PlatformURL == "" {
			t.Errorf("%s: incomplete result %+v", platform, res)
		}
		if res.ChannelID != "" {
			t.Errorf("%s: placeholder channel should not report an ID, got %q", platform, res.ChannelID)
		}
		want := "would publish " + platform + " " + post.ID.String() + " Launch day #go"
		if !strings.Contains(buf.String(), want) {
			t.Errorf("%s: log missing %q\ngot: %s", platform, want, buf.String())
		}
	}
}
