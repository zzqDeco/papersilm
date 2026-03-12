package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"papersilm/pkg/protocol"
)

type Store struct {
	baseDir string
}

func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func (s *Store) BaseDir() string {
	return s.baseDir
}

func (s *Store) Ensure() error {
	for _, dir := range []string{
		filepath.Join(s.baseDir, "sessions"),
		filepath.Join(s.baseDir, "output-styles"),
		filepath.Join(s.baseDir, "skills"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SessionsDir() string {
	return filepath.Join(s.baseDir, "sessions")
}

func (s *Store) SessionDir(sessionID string) string {
	return filepath.Join(s.SessionsDir(), sessionID)
}

func (s *Store) sessionPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "session.json")
}

func (s *Store) sourcesPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "sources.json")
}

func (s *Store) planPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "plan.json")
}

func (s *Store) digestsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "digests")
}

func (s *Store) artifactsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "artifacts")
}

func (s *Store) eventsPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "events.jsonl")
}

func (s *Store) CreateSession(meta protocol.SessionMeta) error {
	sessionDir := s.SessionDir(meta.SessionID)
	for _, dir := range []string{sessionDir, s.digestsDir(meta.SessionID), s.artifactsDir(meta.SessionID), filepath.Join(sessionDir, "cache")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return s.saveJSON(s.sessionPath(meta.SessionID), meta)
}

func (s *Store) SaveMeta(meta protocol.SessionMeta) error {
	return s.saveJSON(s.sessionPath(meta.SessionID), meta)
}

func (s *Store) LoadMeta(sessionID string) (protocol.SessionMeta, error) {
	var meta protocol.SessionMeta
	if err := s.loadJSON(s.sessionPath(sessionID), &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func (s *Store) SaveSources(sessionID string, refs []protocol.PaperRef) error {
	return s.saveJSON(s.sourcesPath(sessionID), refs)
}

func (s *Store) LoadSources(sessionID string) ([]protocol.PaperRef, error) {
	var refs []protocol.PaperRef
	err := s.loadJSON(s.sourcesPath(sessionID), &refs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return refs, err
}

func (s *Store) SavePlan(sessionID string, plan protocol.PlanResult) error {
	return s.saveJSON(s.planPath(sessionID), plan)
}

func (s *Store) LoadPlan(sessionID string) (*protocol.PlanResult, error) {
	var plan protocol.PlanResult
	err := s.loadJSON(s.planPath(sessionID), &plan)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *Store) SaveDigest(sessionID string, digest protocol.PaperDigest) error {
	return s.saveJSON(filepath.Join(s.digestsDir(sessionID), digest.PaperID+".json"), digest)
}

func (s *Store) LoadDigests(sessionID string) ([]protocol.PaperDigest, error) {
	entries, err := os.ReadDir(s.digestsDir(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]protocol.PaperDigest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var digest protocol.PaperDigest
		if err := s.loadJSON(filepath.Join(s.digestsDir(sessionID), entry.Name()), &digest); err != nil {
			return nil, err
		}
		out = append(out, digest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PaperID < out[j].PaperID
	})
	return out, nil
}

func (s *Store) SaveComparison(sessionID string, cmp protocol.ComparisonDigest) error {
	return s.saveJSON(filepath.Join(s.artifactsDir(sessionID), "comparison.json"), cmp)
}

func (s *Store) LoadComparison(sessionID string) (*protocol.ComparisonDigest, error) {
	var cmp protocol.ComparisonDigest
	err := s.loadJSON(filepath.Join(s.artifactsDir(sessionID), "comparison.json"), &cmp)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cmp, nil
}

func (s *Store) SaveArtifactManifest(sessionID string, manifest protocol.ArtifactManifest) error {
	return s.saveJSON(filepath.Join(s.artifactsDir(sessionID), manifest.ArtifactID+".manifest.json"), manifest)
}

func (s *Store) LoadArtifactManifests(sessionID string) ([]protocol.ArtifactManifest, error) {
	entries, err := os.ReadDir(s.artifactsDir(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ArtifactManifest, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if len(name) < len(".manifest.json") || name[len(name)-len(".manifest.json"):] != ".manifest.json" {
			continue
		}
		var manifest protocol.ArtifactManifest
		if err := s.loadJSON(filepath.Join(s.artifactsDir(sessionID), name), &manifest); err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) AppendEvent(sessionID string, event protocol.StreamEvent) error {
	if err := os.MkdirAll(s.SessionDir(sessionID), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.eventsPath(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(raw))
	return err
}

func (s *Store) Snapshot(sessionID string) (protocol.SessionSnapshot, error) {
	meta, err := s.LoadMeta(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	sources, err := s.LoadSources(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	plan, err := s.LoadPlan(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	digests, err := s.LoadDigests(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	cmp, err := s.LoadComparison(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	artifacts, err := s.LoadArtifactManifests(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return protocol.SessionSnapshot{
		Meta:      meta,
		Sources:   sources,
		Plan:      plan,
		Digests:   digests,
		Compare:   cmp,
		Artifacts: artifacts,
	}, nil
}

func (s *Store) LatestSessionID() (string, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if err != nil {
		return "", err
	}
	type candidate struct {
		id  string
		mod time.Time
	}
	var latest candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if info.ModTime().After(latest.mod) {
			latest = candidate{id: entry.Name(), mod: info.ModTime()}
		}
	}
	if latest.id == "" {
		return "", os.ErrNotExist
	}
	return latest.id, nil
}

func (s *Store) saveJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *Store) loadJSON(path string, v interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
