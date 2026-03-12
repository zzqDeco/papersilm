package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Service struct {
	config config.Config
}

func New(cfg config.Config) *Service {
	return &Service{config: cfg}
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
	id := buildPaperID(sessionID, idx, trimmed)
	ref := protocol.PaperRef{
		PaperID:    id,
		URI:        trimmed,
		Label:      defaultLabel(trimmed),
		Status:     protocol.SourceStatusAttached,
		SourceType: protocol.SourceTypeLocalPDF,
	}
	switch {
	case isArxivAbs(trimmed):
		ref.SourceType = protocol.SourceTypeArxivAbs
	case isArxivPDF(trimmed):
		ref.SourceType = protocol.SourceTypeArxivPDF
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

func defaultLabel(uri string) string {
	if isArxivAbs(uri) || isArxivPDF(uri) {
		return "arxiv"
	}
	return filepath.Base(uri)
}

var (
	arxivAbsPattern = regexp.MustCompile(`^https?://arxiv\.org/abs/([^/?#]+)$`)
	arxivPDFPattern = regexp.MustCompile(`^https?://arxiv\.org/pdf/([^/?#]+)(\.pdf)?$`)
)

func isArxivAbs(in string) bool {
	return arxivAbsPattern.MatchString(in)
}

func isArxivPDF(in string) bool {
	return arxivPDFPattern.MatchString(in)
}

func canonicalArxivPDF(in string) (string, error) {
	if m := arxivAbsPattern.FindStringSubmatch(in); len(m) == 2 {
		return "https://arxiv.org/pdf/" + m[1] + ".pdf", nil
	}
	if m := arxivPDFPattern.FindStringSubmatch(in); len(m) >= 2 {
		return "https://arxiv.org/pdf/" + m[1] + ".pdf", nil
	}
	return "", fmt.Errorf("not an arxiv source: %s", in)
}

func sortDigests(digests []protocol.PaperDigest) {
	sort.Slice(digests, func(i, j int) bool {
		return digests[i].PaperID < digests[j].PaperID
	})
}
