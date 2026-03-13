package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Service struct {
	config       config.Config
	httpClient   *http.Client
	arxivBaseURL string
	alphaXiv     *AlphaXivClient
}

func New(cfg config.Config) *Service {
	client := http.DefaultClient
	return &Service{
		config:       cfg,
		httpClient:   client,
		arxivBaseURL: "https://arxiv.org",
		alphaXiv:     NewAlphaXivClient("https://alphaxiv.org", client),
	}
}

func (s *Service) NormalizeSources(_ context.Context, sessionID string, raw []string) ([]protocol.PaperRef, error) {
	out := make([]protocol.PaperRef, 0, len(raw))
	for idx, src := range raw {
		ref, err := s.normalizeSource(sessionID, idx, src)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

func (s *Service) normalizeSource(sessionID string, idx int, raw string) (protocol.PaperRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return protocol.PaperRef{}, fmt.Errorf("empty source")
	}
	ref := protocol.PaperRef{
		URI:                    trimmed,
		Status:                 protocol.SourceStatusAttached,
		SourceType:             protocol.SourceTypeLocalPDF,
		PreferredContentSource: protocol.ContentSourceUnknown,
		ContentProvenance:      protocol.ContentSourceUnknown,
	}
	switch {
	case isPaperID(trimmed):
		ref.SourceType = protocol.SourceTypePaperID
		ref.ResolvedPaperID = trimmed
		ref.PreferredContentSource = protocol.ContentSourceAlphaXivOverview
	case isArxivAbs(trimmed):
		ref.SourceType = protocol.SourceTypeArxivAbs
		ref.ResolvedPaperID = mustExtractPaperID(trimmed)
		ref.PreferredContentSource = protocol.ContentSourceAlphaXivOverview
	case isArxivPDF(trimmed):
		ref.SourceType = protocol.SourceTypeArxivPDF
		ref.ResolvedPaperID = mustExtractPaperID(trimmed)
		ref.PreferredContentSource = protocol.ContentSourceAlphaXivOverview
	case isAlphaXivOverview(trimmed):
		ref.SourceType = protocol.SourceTypeAlphaXivOverview
		ref.ResolvedPaperID = mustExtractPaperID(trimmed)
		ref.PreferredContentSource = protocol.ContentSourceAlphaXivOverview
	case isAlphaXivAbs(trimmed):
		ref.SourceType = protocol.SourceTypeAlphaXivAbs
		ref.ResolvedPaperID = mustExtractPaperID(trimmed)
		ref.PreferredContentSource = protocol.ContentSourceAlphaXivFullText
	default:
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return protocol.PaperRef{}, err
		}
		if strings.ToLower(filepath.Ext(abs)) != ".pdf" {
			return protocol.PaperRef{}, fmt.Errorf("only pdf sources are supported in v1: %s", trimmed)
		}
		if _, err := os.Stat(abs); err != nil {
			return protocol.PaperRef{}, err
		}
		ref.URI = abs
		ref.LocalPath = abs
	}
	idBase := trimmed
	if ref.ResolvedPaperID != "" {
		idBase = ref.ResolvedPaperID
	}
	ref.PaperID = buildPaperID(sessionID, idx, idBase)
	ref.Label = defaultLabel(ref)
	return ref, nil
}

func buildPaperID(sessionID string, idx int, uri string) string {
	replacer := strings.NewReplacer("https://", "", "http://", "", "/", "_", ".", "_", ":", "_", "-", "_")
	base := replacer.Replace(uri)
	if len(base) > 36 {
		base = base[:36]
	}
	return fmt.Sprintf("%s_%02d_%s", sessionID[len(sessionID)-6:], idx+1, base)
}

func defaultLabel(ref protocol.PaperRef) string {
	if ref.ResolvedPaperID != "" {
		return ref.ResolvedPaperID
	}
	return filepath.Base(ref.URI)
}

var (
	paperIDPattern          = regexp.MustCompile(`^(?:\d{4}\.\d{4,5}|[a-z\-]+(?:\.[A-Za-z\-]+)?/\d{7})(?:v\d+)?$`)
	arxivAbsPattern         = regexp.MustCompile(`^https?://arxiv\.org/abs/([^/?#]+)$`)
	arxivPDFPattern         = regexp.MustCompile(`^https?://arxiv\.org/pdf/([^/?#]+?)(?:\.pdf)?$`)
	alphaXivOverviewPattern = regexp.MustCompile(`^https?://alphaxiv\.org/overview/([^/?#]+?)(?:\.md)?$`)
	alphaXivAbsPattern      = regexp.MustCompile(`^https?://alphaxiv\.org/abs/([^/?#]+?)(?:\.md)?$`)
)

func isPaperID(in string) bool {
	return paperIDPattern.MatchString(strings.TrimSpace(in))
}

func isArxivAbs(in string) bool {
	return arxivAbsPattern.MatchString(in)
}

func isArxivPDF(in string) bool {
	return arxivPDFPattern.MatchString(in)
}

func isAlphaXivOverview(in string) bool {
	return alphaXivOverviewPattern.MatchString(in)
}

func isAlphaXivAbs(in string) bool {
	return alphaXivAbsPattern.MatchString(in)
}

func mustExtractPaperID(in string) string {
	switch {
	case isPaperID(in):
		return strings.TrimSpace(in)
	case isArxivAbs(in):
		return arxivAbsPattern.FindStringSubmatch(in)[1]
	case isArxivPDF(in):
		return arxivPDFPattern.FindStringSubmatch(in)[1]
	case isAlphaXivOverview(in):
		return alphaXivOverviewPattern.FindStringSubmatch(in)[1]
	case isAlphaXivAbs(in):
		return alphaXivAbsPattern.FindStringSubmatch(in)[1]
	default:
		return ""
	}
}

func canonicalArxivPDF(baseURL string, ref protocol.PaperRef) (string, error) {
	paperID := strings.TrimSpace(ref.ResolvedPaperID)
	if paperID == "" {
		paperID = mustExtractPaperID(ref.URI)
	}
	if paperID == "" {
		return "", fmt.Errorf("not an arxiv-compatible source: %s", ref.URI)
	}
	return strings.TrimRight(baseURL, "/") + "/pdf/" + paperID + ".pdf", nil
}

func sortDigests(digests []protocol.PaperDigest) {
	sort.Slice(digests, func(i, j int) bool {
		return digests[i].PaperID < digests[j].PaperID
	})
}
