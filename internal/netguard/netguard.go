// Package netguard fetches user-supplied URLs (source pages, post images,
// video thumbnails) without becoming an SSRF proxy into the private network.
//
// The check runs in the dialer, after DNS resolution and for every connection
// including redirects, so a public hostname that resolves (or re-resolves) to
// 127.0.0.1, 10.0.0.0/8 or the cloud metadata address is refused.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a URL resolves to a non-public address.
var ErrBlockedAddress = errors.New("netguard: destination address is not public")

// maxRedirects bounds redirect chains on guarded clients.
const maxRedirects = 5

// blockedPrefixes are special-purpose ranges that are not covered by the
// netip predicates used in IsPublic.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 (can embed private IPv4)
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// IsPublic reports whether addr is a globally routable unicast address.
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// CheckURL rejects anything but absolute http(s) URLs without credentials.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("netguard: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("netguard: scheme %q not allowed", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("netguard: URL has no host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("netguard: URLs with credentials are not allowed")
	}
	return u, nil
}

// NewClient returns an HTTP client for user-supplied URLs. Unless
// allowPrivate is set (development against a local MinIO, for example),
// connections to non-public addresses fail with ErrBlockedAddress. The client
// ignores HTTP_PROXY so the check applies to the real destination.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !IsPublic(ap.Addr()) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, address)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          10,
			IdleConnTimeout:       60 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("netguard: too many redirects")
			}
			_, err := CheckURL(req.URL.String())
			return err
		},
	}
}

// Fetch GETs rawURL with client and returns at most maxBytes of body. It
// fails on non-200 responses and on bodies larger than maxBytes instead of
// truncating them.
func Fetch(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, string, error) {
	if _, err := CheckURL(rawURL); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("netguard: GET returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("netguard: response is %d bytes, limit %d", resp.ContentLength, maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > maxBytes {
		return nil, "", fmt.Errorf("netguard: response exceeds %d bytes", maxBytes)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
