package netguard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestIsPublic(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"0.0.0.0", "100.64.0.1", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1",
		"::ffff:169.254.169.254", "224.0.0.1", "64:ff9b::a00:1", "198.18.0.1",
	}
	for _, s := range blocked {
		if IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s classified as public", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s classified as non-public", s)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "gopher://x", "http://", "https://user:pw@example.com/", "ftp://example.com", "//example.com/x"} {
		if _, err := CheckURL(bad); err == nil {
			t.Errorf("CheckURL(%q) accepted", bad)
		}
	}
	if _, err := CheckURL("https://example.com/a.png"); err != nil {
		t.Errorf("valid URL rejected: %v", err)
	}
}

func TestFetch_BlocksLoopbackAndRedirectsToIt(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	defer internal.Close()

	client := NewClient(5*time.Second, false)
	_, _, err := Fetch(context.Background(), client, internal.URL, 1024)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback fetch: err = %v, want ErrBlockedAddress", err)
	}
	// Hostnames are checked after resolution, not by string.
	_, _, err = Fetch(context.Background(), client, strings.Replace(internal.URL, "127.0.0.1", "localhost", 1), 1024)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("localhost fetch: err = %v, want ErrBlockedAddress", err)
	}
}

func TestFetch_AllowPrivateAndSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()

	client := NewClient(5*time.Second, true)
	body, ctype, err := Fetch(context.Background(), client, srv.URL+"/ok", 1024)
	if err != nil || string(body) != "png" || ctype != "image/png" {
		t.Fatalf("Fetch = %q, %q, %v", body, ctype, err)
	}
	if _, _, err := Fetch(context.Background(), client, srv.URL+"/big", 1024); err == nil {
		t.Fatal("oversized body accepted")
	}
	if _, _, err := Fetch(context.Background(), client, srv.URL+"/redirect", 1024); err == nil {
		t.Fatal("redirect to file:// followed")
	}
}
