package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/html"

	"github.com/himanshumehta/social-publisher/internal/model"
)

const (
	scanCacheTTL     = 24 * time.Hour
	scanCachePrefix  = "post:scan:"
	scanHTTPTimeout  = 15 * time.Second
	maxBodyBytes     = 5 * 1024 * 1024
	maxBodyTextChars = 10000
)

type PostScanner struct {
	pool   *pgxpool.Pool
	rdb    *redis.Client
	logger *slog.Logger
	client *http.Client
}

func NewPostScanner(pool *pgxpool.Pool, rdb *redis.Client, logger *slog.Logger) *PostScanner {
	return &PostScanner{
		pool:   pool,
		rdb:    rdb,
		logger: logger,
		client: &http.Client{
			Timeout: scanHTTPTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (s *PostScanner) Scan(ctx context.Context, rawURL string) (*model.ScannedContent, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid URL: %s", rawURL)
	}

	cacheKey := scanCachePrefix + hashURL(rawURL)
	cached, err := s.rdb.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var content model.ScannedContent
		if json.Unmarshal(cached, &content) == nil {
			return &content, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "SocialPublisherBot/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("URL returned status %d", resp.StatusCode)
	}

	body := io.LimitReader(resp.Body, maxBodyBytes)

	doc, err := html.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	content := s.extractContent(doc, rawURL)
	content.ScannedAt = time.Now().UTC()

	if data, err := json.Marshal(content); err == nil {
		s.rdb.Set(ctx, cacheKey, data, scanCacheTTL)
	}

	return content, nil
}

func (s *PostScanner) extractContent(doc *html.Node, sourceURL string) *model.ScannedContent {
	content := &model.ScannedContent{
		OGTags: make(map[string]string),
	}

	s.extractMeta(doc, content)
	content.BodyText = s.extractBodyText(doc)
	content.WordCount = countWords(content.BodyText)
	content.Images = s.extractImages(doc, sourceURL)

	if content.Title == "" {
		content.Title = content.OGTags["og:title"]
	}
	if content.Title == "" {
		content.Title = s.extractTitle(doc)
	}
	if content.Description == "" {
		content.Description = content.OGTags["og:description"]
	}
	if content.CoverImageURL == "" {
		content.CoverImageURL = content.OGTags["og:image"]
	}
	if content.SiteName == "" {
		content.SiteName = content.OGTags["og:site_name"]
	}
	if content.Author == "" {
		content.Author = content.OGTags["article:author"]
	}

	if utf8.RuneCountInString(content.BodyText) > maxBodyTextChars {
		runes := []rune(content.BodyText)
		content.BodyText = string(runes[:maxBodyTextChars])
	}

	return content
}

func (s *PostScanner) extractMeta(n *html.Node, content *model.ScannedContent) {
	if n.Type == html.ElementNode && n.Data == "meta" {
		var property, name, metaContent string
		for _, attr := range n.Attr {
			switch attr.Key {
			case "property":
				property = attr.Val
			case "name":
				name = attr.Val
			case "content":
				metaContent = attr.Val
			}
		}

		key := property
		if key == "" {
			key = name
		}

		if key != "" && metaContent != "" {
			content.OGTags[key] = metaContent

			switch key {
			case "description":
				if content.Description == "" {
					content.Description = metaContent
				}
			case "author":
				content.Author = metaContent
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.extractMeta(c, content)
	}
}

func (s *PostScanner) extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if n.FirstChild != nil {
			return strings.TrimSpace(n.FirstChild.Data)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := s.extractTitle(c); title != "" {
			return title
		}
	}
	return ""
}

func (s *PostScanner) extractBodyText(n *html.Node) string {
	var sb strings.Builder
	s.walkText(n, &sb)
	return strings.TrimSpace(sb.String())
}

var skipElements = map[string]bool{
	"script": true, "style": true, "nav": true, "footer": true,
	"header": true, "aside": true, "noscript": true, "iframe": true,
	"svg": true, "form": true,
}

func (s *PostScanner) walkText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode && skipElements[n.Data] {
		return
	}
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.walkText(c, sb)
	}
}

func (s *PostScanner) extractImages(n *html.Node, baseURL string) []string {
	var images []string
	s.walkImages(n, &images, baseURL)
	seen := make(map[string]bool)
	var unique []string
	for _, img := range images {
		if !seen[img] {
			seen[img] = true
			unique = append(unique, img)
		}
	}
	return unique
}

func (s *PostScanner) walkImages(n *html.Node, images *[]string, baseURL string) {
	if n.Type == html.ElementNode && n.Data == "img" {
		for _, attr := range n.Attr {
			if attr.Key == "src" && attr.Val != "" {
				imgURL := resolveURL(baseURL, attr.Val)
				if imgURL != "" {
					*images = append(*images, imgURL)
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.walkImages(c, images, baseURL)
	}
}

func hashURL(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:16])
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		return "https:" + ref
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ""
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return baseURL.ResolveReference(refURL).String()
}
