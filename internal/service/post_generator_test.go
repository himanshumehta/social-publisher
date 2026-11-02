package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestValidateGenerated(t *testing.T) {
	posts := map[string]*model.PlatformPost{
		"twitter":  {Text: strings.Repeat("é", 300), CharCount: 10}, // model under-reports
		"linkedin": {Text: "fine", Hashtags: []string{"go"}},
		"myspace":  {Text: "not requested"},
	}
	got, err := validateGenerated(posts, []string{"twitter", "linkedin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["myspace"]; ok {
		t.Error("unrequested platform kept")
	}
	if got["twitter"].CharCount != 300 {
		t.Errorf("char count = %d, want 300 (runes, recomputed)", got["twitter"].CharCount)
	}
	if got["linkedin"].CharCount != len("fine")+len("go")+2 || got["linkedin"].Platform != "linkedin" {
		t.Errorf("linkedin = %+v", got["linkedin"])
	}

	// The reviewer now sees the real length and flags the over-long tweet
	// for the admin.
	review := NewPostReviewer(slog.Default()).Review(context.Background(), &model.Post{Title: "t"}, got)
	if !strings.Contains(review.Notes, "char_limit_exceeded") {
		t.Errorf("reviewer did not flag a 300-char tweet: %q", review.Notes)
	}

	for name, in := range map[string]map[string]*model.PlatformPost{
		"missing platform": {"twitter": {Text: "x"}},
		"empty text":       {"twitter": {Text: "x"}, "linkedin": {Text: "  "}},
	} {
		if _, err := validateGenerated(in, []string{"twitter", "linkedin"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGenerate_FailsFastWithoutKey(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()
	g := NewPostGenerator(nil, rdb, slog.Default(), "")
	_, err := g.Generate(context.Background(), &GenerateRequest{ManualTitle: "x", Platforms: []string{"twitter"}})
	if !errors.Is(err, ErrNoGenerationKey) {
		t.Fatalf("err = %v, want ErrNoGenerationKey", err)
	}
}
