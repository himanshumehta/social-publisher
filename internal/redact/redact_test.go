package redact

import (
	"bytes"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	cases := map[string]string{
		`Get "https://graph.facebook.com/v21.0/me?fields=id&access_token=EAAB123secret": dial tcp: timeout`: `access_token=[REDACTED]"`,
		"upload_phase=start&access_token=EAAsecret&file_size=10":                                            "access_token=[REDACTED]&file_size=10",
		"grant_type=th_exchange_token&client_secret=abc123&access_token=xyz":                                "client_secret=[REDACTED]&access_token=[REDACTED]",
		`{"access_token":"ya29.secret","expires_in":3599}`:                                                  `{"access_token":"[REDACTED]","expires_in":3599}`,
		"Authorization: Bearer abcdefghijklmnop":                                                            "Bearer [REDACTED]",
		"callback?code=4/0Aabc&state=x":                                                                     "code=[REDACTED]&state=x",
	}
	for in, want := range cases {
		got := String(in)
		if !strings.Contains(got, want) {
			t.Errorf("String(%q) = %q, want it to contain %q", in, got, want)
		}
		for _, secret := range []string{"EAAB123secret", "EAAsecret", "abc123", "ya29.secret", "abcdefghijklmnop", "4/0Aabc"} {
			if strings.Contains(got, secret) {
				t.Errorf("String(%q) leaked %q", in, secret)
			}
		}
	}
	if got := String("published 3 posts, retry_count=1"); got != "published 3 posts, retry_count=1" {
		t.Errorf("benign text changed: %q", got)
	}
}

func TestReplaceAttr_RedactsURLErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: ReplaceAttr}))
	err := &url.Error{Op: "Get", URL: "https://graph.threads.net/v1.0/me?access_token=THREADS-SECRET", Err: errors.New("timeout")}
	logger.Error("refresh failed", "error", err, "url", "https://x.test/?refresh_token=RT-SECRET")
	out := buf.String()
	if strings.Contains(out, "THREADS-SECRET") || strings.Contains(out, "RT-SECRET") {
		t.Fatalf("token leaked into log: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %s", out)
	}
}
