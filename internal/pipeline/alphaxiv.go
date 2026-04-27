package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

var ErrAlphaXivNotFound = errors.New("alphaxiv content not found")

type AlphaXivClient struct {
	baseURL    string
	httpClient *http.Client
}

type alphaSection struct {
	Title string
	Body  string
}

func NewAlphaXivClient(baseURL string, httpClient *http.Client) *AlphaXivClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AlphaXivClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *AlphaXivClient) FetchOverview(ctx context.Context, paperID string) (string, error) {
	return c.fetchMarkdown(ctx, "overview", paperID)
}

func (c *AlphaXivClient) FetchFullText(ctx context.Context, paperID string) (string, error) {
	return c.fetchMarkdown(ctx, "abs", paperID)
}

func (c *AlphaXivClient) fetchMarkdown(ctx context.Context, kind, paperID string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s.md", c.baseURL, kind, paperID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", ErrAlphaXivNotFound
	case resp.StatusCode >= 300:
		return "", fmt.Errorf("fetch %s failed: %s", kind, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func supportsAlphaXiv(ref protocol.PaperRef) bool {
	return strings.TrimSpace(ref.ResolvedPaperID) != ""
}

func (s *Service) LookupAlphaXivOverview(ctx context.Context, sessionID string, ref protocol.PaperRef) (string, bool, error) {
	return s.lookupAlphaXivMarkdown(ctx, sessionID, ref, protocol.ContentSourceAlphaXivOverview)
}

func (s *Service) LookupAlphaXivFullText(ctx context.Context, sessionID string, ref protocol.PaperRef) (string, bool, error) {
	return s.lookupAlphaXivMarkdown(ctx, sessionID, ref, protocol.ContentSourceAlphaXivFullText)
}

func (s *Service) lookupAlphaXivMarkdown(ctx context.Context, sessionID string, ref protocol.PaperRef, source protocol.ContentSource) (string, bool, error) {
	if !supportsAlphaXiv(ref) {
		return "", false, nil
	}
	if cached, found, missing, err := s.readAlphaXivCache(sessionID, ref.ResolvedPaperID, source); err != nil {
		return "", false, err
	} else if found {
		return cached, true, nil
	} else if missing {
		return "", false, nil
	}

	var (
		content string
		err     error
	)
	switch source {
	case protocol.ContentSourceAlphaXivOverview:
		content, err = s.alphaXiv.FetchOverview(ctx, ref.ResolvedPaperID)
	case protocol.ContentSourceAlphaXivFullText:
		content, err = s.alphaXiv.FetchFullText(ctx, ref.ResolvedPaperID)
	default:
		return "", false, fmt.Errorf("unsupported alphaxiv source: %s", source)
	}
	if errors.Is(err, ErrAlphaXivNotFound) {
		if markErr := s.writeAlphaXivMissingCache(sessionID, ref.ResolvedPaperID, source); markErr != nil {
			return "", false, markErr
		}
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if err := s.writeAlphaXivCache(sessionID, ref.ResolvedPaperID, source, content); err != nil {
		return "", false, err
	}
	return content, true, nil
}

func (s *Service) readAlphaXivCache(sessionID, paperID string, source protocol.ContentSource) (content string, found, missing bool, err error) {
	mdPath, missPath := s.alphaXivCachePaths(sessionID, paperID, source)
	if raw, readErr := os.ReadFile(mdPath); readErr == nil {
		return string(raw), true, false, nil
	} else if !os.IsNotExist(readErr) {
		return "", false, false, readErr
	}
	if _, statErr := os.Stat(missPath); statErr == nil {
		return "", false, true, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, false, statErr
	}
	return "", false, false, nil
}

func (s *Service) writeAlphaXivCache(sessionID, paperID string, source protocol.ContentSource, content string) error {
	mdPath, missPath := s.alphaXivCachePaths(sessionID, paperID, source)
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(missPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(mdPath, []byte(content), 0o644)
}

func (s *Service) writeAlphaXivMissingCache(sessionID, paperID string, source protocol.ContentSource) error {
	mdPath, missPath := s.alphaXivCachePaths(sessionID, paperID, source)
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(missPath, []byte("missing"), 0o644)
}

func (s *Service) alphaXivCachePaths(sessionID, paperID string, source protocol.ContentSource) (string, string) {
	_ = sessionID
	cacheDir := filepath.Join(s.config.BaseDir, "cache", "alphaxiv")
	name := sanitizeCacheName(paperID + "." + string(source))
	return filepath.Join(cacheDir, name+".md"), filepath.Join(cacheDir, name+".missing")
}

func sanitizeCacheName(in string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "?", "_", "&", "_", "=", "_")
	return replacer.Replace(in)
}

var (
	alphaHeadingPattern = regexp.MustCompile(`^\s{0,3}(#{2,6})\s+(.*)$`)
	alphaTitlePattern   = regexp.MustCompile(`(?i)research paper analysis:\s*"?([^"]+?)"?$`)
	numberingPrefix     = regexp.MustCompile(`^\d+(\.\d+)*\s*[:.)-]?\s*`)
)

func parseAlphaSections(markdown string) []alphaSection {
	lines := strings.Split(markdown, "\n")
	sections := make([]alphaSection, 0, 8)
	current := alphaSection{}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if m := alphaHeadingPattern.FindStringSubmatch(line); len(m) == 3 {
			if strings.TrimSpace(current.Title) != "" || strings.TrimSpace(current.Body) != "" {
				current.Body = strings.TrimSpace(current.Body)
				sections = append(sections, current)
			}
			current = alphaSection{Title: strings.TrimSpace(m[2])}
			continue
		}
		if current.Body != "" {
			current.Body += "\n"
		}
		current.Body += line
	}
	if strings.TrimSpace(current.Title) != "" || strings.TrimSpace(current.Body) != "" {
		current.Body = strings.TrimSpace(current.Body)
		sections = append(sections, current)
	}
	return sections
}

func extractAlphaTitle(markdown string) string {
	for _, section := range parseAlphaSections(markdown) {
		title := strings.TrimSpace(section.Title)
		if title == "" {
			continue
		}
		if m := alphaTitlePattern.FindStringSubmatch(title); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
		if !strings.Contains(strings.ToLower(title), "research paper analysis") {
			return cleanAlphaHeading(title)
		}
	}
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" || strings.Contains(strings.ToLower(line), "provided proper attribution") {
			continue
		}
		if isLikelyHeading(line) {
			return cleanAlphaHeading(line)
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if len(line) >= 8 {
			return cleanAlphaHeading(line)
		}
	}
	return ""
}

func cleanAlphaHeading(in string) string {
	in = numberingPrefix.ReplaceAllString(strings.TrimSpace(in), "")
	in = strings.Trim(in, "\"")
	return strings.TrimSpace(in)
}

func classifyAlphaSection(title string) string {
	l := strings.ToLower(cleanAlphaHeading(title))
	switch {
	case strings.Contains(l, "method") || strings.Contains(l, "architecture") || strings.Contains(l, "approach"):
		return "method"
	case strings.Contains(l, "experimental") || strings.Contains(l, "experiment") || strings.Contains(l, "evaluation"):
		return "experiment"
	case strings.Contains(l, "result") || strings.Contains(l, "finding"):
		return "results"
	case strings.Contains(l, "conclusion") || strings.Contains(l, "takeaway"):
		return "conclusion"
	case strings.Contains(l, "limitation") || strings.Contains(l, "caveat"):
		return "limitations"
	case strings.Contains(l, "objective") || strings.Contains(l, "motivation") || strings.Contains(l, "problem"):
		return "abstract"
	case strings.Contains(l, "significance") || strings.Contains(l, "broader") || strings.Contains(l, "landscape") || strings.Contains(l, "context") || strings.Contains(l, "author"):
		return "background"
	default:
		return "body"
	}
}

func alphaMarkdownToChunks(markdown string) []chunk {
	sections := parseAlphaSections(markdown)
	out := make([]chunk, 0, len(sections))
	for _, section := range sections {
		body := strings.TrimSpace(section.Body)
		if body == "" {
			continue
		}
		out = append(out, chunk{
			Page:    0,
			Section: classifyAlphaSection(section.Title),
			Content: body,
		})
	}
	if len(out) == 0 && strings.TrimSpace(markdown) != "" {
		return []chunk{{Page: 0, Section: "body", Content: markdown}}
	}
	return out
}

func extractAlphaSectionHints(markdown string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	for _, section := range parseAlphaSections(markdown) {
		label := classifyAlphaSection(section.Title)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func alphaIntroSnippet(markdown string) string {
	chunks := alphaMarkdownToChunks(markdown)
	for _, item := range chunks {
		if item.Section == "abstract" || item.Section == "method" || item.Section == "experiment" {
			return trimForLine(firstSentence(item.Content))
		}
	}
	return trimForLine(firstSentence(markdown))
}

func hasEnoughMarkdownText(markdown string) bool {
	text := strings.TrimSpace(markdown)
	return len(text) > 600
}
