package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"

	"github.com/zzqDeco/papersilm/internal/taskboard"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type Store struct {
	baseDir       string
	workspaceRoot string
}

type workspaceState struct {
	PaperID     string                     `json:"paper_id"`
	Notes       []protocol.PaperNote       `json:"notes,omitempty"`
	Annotations []protocol.PaperAnnotation `json:"annotations,omitempty"`
	Similar     []protocol.SimilarPaperRef `json:"similar,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

func New(baseDir string) *Store {
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	workspaceRoot := baseDir
	if filepath.Base(baseDir) == ".papersilm" {
		workspaceRoot = filepath.Dir(baseDir)
	}
	return &Store{
		baseDir:       baseDir,
		workspaceRoot: workspaceRoot,
	}
}

func (s *Store) BaseDir() string {
	return s.baseDir
}

func (s *Store) WorkspaceRoot() string {
	return s.workspaceRoot
}

func (s *Store) Ensure() error {
	for _, dir := range []string{
		filepath.Join(s.baseDir, "sessions"),
		filepath.Join(s.baseDir, "index"),
		filepath.Join(s.baseDir, "cache", "papers"),
		filepath.Join(s.baseDir, "cache", "alphaxiv"),
		filepath.Join(s.baseDir, "cache", "pages"),
		filepath.Join(s.baseDir, "ops", "commands"),
		filepath.Join(s.baseDir, "output-styles"),
		filepath.Join(s.baseDir, "skills"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return s.RefreshWorkspaceState()
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

func (s *Store) executionStatePath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "execution_state.json")
}

func (s *Store) digestsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "digests")
}

func (s *Store) artifactsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "artifacts")
}

func (s *Store) checkpointsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "checkpoints")
}

func (s *Store) workspacesDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "workspaces")
}

func (s *Store) workspacePath(sessionID, paperID string) string {
	return filepath.Join(s.workspacesDir(sessionID), paperID+".json")
}

func (s *Store) skillRunsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "skill-runs")
}

func (s *Store) skillRunPath(sessionID, runID string) string {
	return filepath.Join(s.skillRunsDir(sessionID), runID+".json")
}

func (s *Store) skillArtifactsDir(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "skill-artifacts")
}

func (s *Store) eventsPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "events.jsonl")
}

func (s *Store) transcriptPath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "transcript.jsonl")
}

func (s *Store) CreateSession(meta protocol.SessionMeta) error {
	sessionDir := s.SessionDir(meta.SessionID)
	for _, dir := range []string{
		sessionDir,
		s.digestsDir(meta.SessionID),
		s.artifactsDir(meta.SessionID),
		s.skillRunsDir(meta.SessionID),
		s.skillArtifactsDir(meta.SessionID),
		s.checkpointsDir(meta.SessionID),
		s.workspacesDir(meta.SessionID),
		filepath.Join(sessionDir, "cache"),
	} {
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
	if len(plan.DAG.Nodes) == 0 && len(plan.Steps) > 0 {
		plan.DAG = legacyStepsToDAG(plan.Steps)
	}
	return s.saveJSON(s.planPath(sessionID), plan)
}

func (s *Store) DeletePlan(sessionID string) error {
	err := os.Remove(s.planPath(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
	if len(plan.DAG.Nodes) == 0 && len(plan.Steps) > 0 {
		plan.DAG = legacyStepsToDAG(plan.Steps)
	}
	return &plan, nil
}

func (s *Store) SaveExecutionState(sessionID string, state protocol.ExecutionState) error {
	return s.saveJSON(s.executionStatePath(sessionID), state)
}

func (s *Store) DeleteExecutionState(sessionID string) error {
	err := os.Remove(s.executionStatePath(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) LoadExecutionState(sessionID string) (*protocol.ExecutionState, error) {
	var state protocol.ExecutionState
	err := s.loadJSON(s.executionStatePath(sessionID), &state)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Store) SaveDigest(sessionID string, digest protocol.PaperDigest) error {
	return s.saveJSON(filepath.Join(s.digestsDir(sessionID), digest.PaperID+".json"), digest)
}

func (s *Store) DeleteDigest(sessionID, paperID string) error {
	err := os.Remove(filepath.Join(s.digestsDir(sessionID), paperID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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

func (s *Store) DeleteComparison(sessionID string) error {
	err := os.Remove(filepath.Join(s.artifactsDir(sessionID), "comparison.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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

func (s *Store) DeleteArtifact(sessionID, artifactID string) error {
	manifestPath := filepath.Join(s.artifactsDir(sessionID), artifactID+".manifest.json")
	var manifest protocol.ArtifactManifest
	err := s.loadJSON(manifestPath, &manifest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		for _, path := range manifest.Paths {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
		}
	} else {
		for _, fallback := range []string{
			filepath.Join(s.artifactsDir(sessionID), artifactID+".md"),
			filepath.Join(s.artifactsDir(sessionID), artifactID+".json"),
		} {
			if removeErr := os.Remove(fallback); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
		}
	}
	if removeErr := os.Remove(manifestPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return nil
}

func (s *Store) DeletePaperDigestArtifacts(sessionID, paperID string) error {
	if err := s.DeleteDigest(sessionID, paperID); err != nil {
		return err
	}
	return s.DeleteArtifact(sessionID, paperID)
}

func (s *Store) DeleteComparisonArtifacts(sessionID string) error {
	if err := s.DeleteComparison(sessionID); err != nil {
		return err
	}
	return s.DeleteArtifact(sessionID, "comparison")
}

func (s *Store) SaveWorkspaceState(sessionID string, workspace protocol.PaperWorkspace) error {
	state := workspaceState{
		PaperID:     workspace.PaperID,
		Notes:       append([]protocol.PaperNote(nil), workspace.Notes...),
		Annotations: append([]protocol.PaperAnnotation(nil), workspace.Annotations...),
		Similar:     append([]protocol.SimilarPaperRef(nil), workspace.Similar...),
		CreatedAt:   workspace.CreatedAt,
		UpdatedAt:   workspace.UpdatedAt,
	}
	return s.saveJSON(s.workspacePath(sessionID, workspace.PaperID), state)
}

func (s *Store) LoadWorkspaceState(sessionID, paperID string) (*protocol.PaperWorkspace, error) {
	var state workspaceState
	err := s.loadJSON(s.workspacePath(sessionID, paperID), &state)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	workspace := protocol.PaperWorkspace{
		PaperID:     state.PaperID,
		Notes:       append([]protocol.PaperNote(nil), state.Notes...),
		Annotations: append([]protocol.PaperAnnotation(nil), state.Annotations...),
		Similar:     append([]protocol.SimilarPaperRef(nil), state.Similar...),
		CreatedAt:   state.CreatedAt,
		UpdatedAt:   state.UpdatedAt,
	}
	return &workspace, nil
}

func (s *Store) LoadWorkspaceStates(sessionID string) ([]protocol.PaperWorkspace, error) {
	entries, err := os.ReadDir(s.workspacesDir(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]protocol.PaperWorkspace, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paperID := strings.TrimSuffix(entry.Name(), ".json")
		workspace, err := s.LoadWorkspaceState(sessionID, paperID)
		if err != nil {
			return nil, err
		}
		if workspace == nil {
			continue
		}
		out = append(out, *workspace)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PaperID < out[j].PaperID
	})
	return out, nil
}

func (s *Store) DeleteWorkspaceState(sessionID, paperID string) error {
	err := os.Remove(s.workspacePath(sessionID, paperID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) DeleteWorkspaceStates(sessionID string, paperIDs []string) error {
	for _, paperID := range paperIDs {
		if strings.TrimSpace(paperID) == "" {
			continue
		}
		if err := s.DeleteWorkspaceState(sessionID, paperID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveSkillRun(sessionID string, run protocol.SkillRunRecord) error {
	return s.saveJSON(s.skillRunPath(sessionID, run.RunID), run)
}

func (s *Store) LoadSkillRun(sessionID, runID string) (*protocol.SkillRunRecord, error) {
	var run protocol.SkillRunRecord
	err := s.loadJSON(s.skillRunPath(sessionID, runID), &run)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Store) LoadSkillRuns(sessionID string) ([]protocol.SkillRunRecord, error) {
	entries, err := os.ReadDir(s.skillRunsDir(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SkillRunRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".json")
		run, err := s.LoadSkillRun(sessionID, runID)
		if err != nil {
			return nil, err
		}
		if run == nil {
			continue
		}
		out = append(out, *run)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].RunID < out[j].RunID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) SaveSkillArtifactManifest(sessionID string, manifest protocol.ArtifactManifest) error {
	return s.saveJSON(filepath.Join(s.skillArtifactsDir(sessionID), manifest.ArtifactID+".manifest.json"), manifest)
}

func (s *Store) LoadSkillArtifactManifests(sessionID string) ([]protocol.ArtifactManifest, error) {
	entries, err := os.ReadDir(s.skillArtifactsDir(sessionID))
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
		if err := s.loadJSON(filepath.Join(s.skillArtifactsDir(sessionID), name), &manifest); err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
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
	return s.appendJSONL(s.eventsPath(sessionID), event)
}

func (s *Store) AppendTranscriptEntry(sessionID string, entry protocol.TranscriptEntry) error {
	return s.appendJSONL(s.transcriptPath(sessionID), entry)
}

func (s *Store) appendJSONL(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(raw))
	return err
}

func (s *Store) LoadRecentEvents(sessionID string, limit int) ([]protocol.StreamEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	events, err := loadJSONL[protocol.StreamEvent](s.eventsPath(sessionID))
	if err != nil {
		return nil, err
	}
	if len(events) <= limit {
		return events, nil
	}
	return append([]protocol.StreamEvent(nil), events[len(events)-limit:]...), nil
}

func (s *Store) LoadTranscript(sessionID string) ([]protocol.TranscriptEntry, error) {
	return loadJSONL[protocol.TranscriptEntry](s.transcriptPath(sessionID))
}

func loadJSONL[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	values := make([]T, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var value T
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			continue
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
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
	execution, err := s.LoadExecutionState(sessionID)
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
	skillRuns, err := s.LoadSkillRuns(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	skillArtifacts, err := s.LoadSkillArtifactManifests(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	visibleSkillRuns, err := s.filterVisibleSkillRuns(sources, skillArtifacts, skillRuns)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	visibleSkillArtifacts := filterVisibleSkillArtifacts(visibleSkillRuns, skillArtifacts)
	workspaces, err := s.LoadWorkspaces(sessionID, meta, sources, digests, artifacts, visibleSkillRuns)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	workspace, err := s.LoadWorkspaceSummary()
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	board := taskboard.Build(meta, plan, execution, artifacts, workspaces, visibleSkillRuns)
	if plan != nil {
		plan.TaskBoard = board
	}
	return protocol.SessionSnapshot{
		Meta:           meta,
		Workspace:      workspace,
		Sources:        sources,
		Plan:           plan,
		TaskBoard:      board,
		Execution:      execution,
		Digests:        digests,
		Compare:        cmp,
		Artifacts:      artifacts,
		SkillRuns:      visibleSkillRuns,
		SkillArtifacts: visibleSkillArtifacts,
		Workspaces:     workspaces,
	}, nil
}

func (s *Store) InvalidatePlanState(sessionID string) error {
	meta, err := s.LoadMeta(sessionID)
	if err != nil {
		return err
	}
	if err := s.DeletePlan(sessionID); err != nil {
		return err
	}
	if err := s.DeleteExecutionState(sessionID); err != nil {
		return err
	}
	if err := os.RemoveAll(s.digestsDir(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.RemoveAll(s.artifactsDir(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.RemoveAll(s.checkpointsDir(sessionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(s.digestsDir(sessionID), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.artifactsDir(sessionID), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.checkpointsDir(sessionID), 0o755); err != nil {
		return err
	}
	meta.ActivePlanID = ""
	meta.ActiveCheckpointID = ""
	meta.PendingInterruptID = ""
	meta.ApprovalPending = false
	sources, loadErr := s.LoadSources(sessionID)
	if loadErr != nil {
		return loadErr
	}
	if len(sources) == 0 {
		meta.State = protocol.SessionStateIdle
	} else {
		meta.State = protocol.SessionStateSourceAttached
	}
	meta.UpdatedAt = time.Now().UTC()
	return s.SaveMeta(meta)
}

func (s *Store) CheckPointStore(sessionID string) adk.CheckPointStore {
	return &fileCheckpointStore{dir: s.checkpointsDir(sessionID)}
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

type fileCheckpointStore struct {
	dir string
}

func (s *fileCheckpointStore) checkpointPath(checkPointID string) string {
	return filepath.Join(s.dir, checkPointID+".bin")
}

func (s *fileCheckpointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	raw, err := os.ReadFile(s.checkpointPath(checkPointID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *fileCheckpointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.checkpointPath(checkPointID), checkPoint, 0o644)
}

func legacyStepsToDAG(steps []protocol.PlanStep) protocol.PlanDAG {
	nodes := make([]protocol.PlanNode, 0, len(steps))
	edges := make([]protocol.PlanEdge, 0, len(steps))
	var previous string
	for idx, step := range steps {
		status := protocol.NodeStatusPending
		if idx == 0 {
			status = protocol.NodeStatusReady
		}
		node := protocol.PlanNode{
			ID:            step.ID,
			Kind:          protocol.NodeKind(step.Tool),
			Goal:          step.Goal,
			PaperIDs:      append([]string(nil), step.PaperIDs...),
			WorkerProfile: protocol.WorkerProfileSupervisor,
			Produces:      []string{step.ExpectedArtifact},
			Required:      true,
			Status:        status,
			ParallelGroup: "legacy_chain",
		}
		if previous != "" {
			node.DependsOn = []string{previous}
			edges = append(edges, protocol.PlanEdge{From: previous, To: step.ID})
		}
		nodes = append(nodes, node)
		previous = step.ID
	}
	return protocol.PlanDAG{Nodes: nodes, Edges: edges}
}

func (s *Store) LoadWorkspaces(sessionID string, meta protocol.SessionMeta, sources []protocol.PaperRef, digests []protocol.PaperDigest, artifacts []protocol.ArtifactManifest, skillRuns []protocol.SkillRunRecord) ([]protocol.PaperWorkspace, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	states, err := s.LoadWorkspaceStates(sessionID)
	if err != nil {
		return nil, err
	}
	stateByPaperID := make(map[string]protocol.PaperWorkspace, len(states))
	for _, state := range states {
		stateByPaperID[state.PaperID] = state
	}
	digestByPaperID := make(map[string]protocol.PaperDigest, len(digests))
	for _, digest := range digests {
		digestByPaperID[digest.PaperID] = digest
	}
	skillRunsByPaperID := make(map[string][]protocol.SkillRunRecord, len(skillRuns))
	for _, run := range skillRuns {
		if run.TargetKind != protocol.SkillTargetKindPaper {
			continue
		}
		skillRunsByPaperID[run.TargetID] = append(skillRunsByPaperID[run.TargetID], run)
	}
	out := make([]protocol.PaperWorkspace, 0, len(sources))
	for _, source := range sources {
		state, hasState := stateByPaperID[source.PaperID]
		workspace := protocol.PaperWorkspace{
			PaperID:     source.PaperID,
			Notes:       []protocol.PaperNote{},
			Annotations: []protocol.PaperAnnotation{},
			Resources:   []protocol.PaperResource{},
			Similar:     []protocol.SimilarPaperRef{},
			SkillRuns:   []protocol.SkillRunRecord{},
			CreatedAt:   meta.CreatedAt,
			UpdatedAt:   meta.UpdatedAt,
		}
		sourceCopy := source
		workspace.Source = &sourceCopy
		if hasState {
			workspace.Notes = append(workspace.Notes, state.Notes...)
			workspace.Annotations = append(workspace.Annotations, state.Annotations...)
			workspace.Similar = append(workspace.Similar, state.Similar...)
			if !state.CreatedAt.IsZero() {
				workspace.CreatedAt = state.CreatedAt
			}
			if !state.UpdatedAt.IsZero() {
				workspace.UpdatedAt = state.UpdatedAt
			}
		}
		if digest, ok := digestByPaperID[source.PaperID]; ok {
			digestCopy := digest
			workspace.Digest = &digestCopy
		}
		workspace.SkillRuns = append(workspace.SkillRuns, skillRunsByPaperID[source.PaperID]...)
		workspace.Resources = buildWorkspaceResources(source, artifacts)
		out = append(out, workspace)
	}
	return out, nil
}

func (s *Store) filterVisibleSkillRuns(sources []protocol.PaperRef, manifests []protocol.ArtifactManifest, runs []protocol.SkillRunRecord) ([]protocol.SkillRunRecord, error) {
	if len(runs) == 0 {
		return nil, nil
	}
	visiblePaperIDs := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		visiblePaperIDs[source.PaperID] = struct{}{}
	}
	manifestByArtifactID := make(map[string]protocol.ArtifactManifest, len(manifests))
	for _, manifest := range manifests {
		manifestByArtifactID[manifest.ArtifactID] = manifest
	}
	out := make([]protocol.SkillRunRecord, 0, len(runs))
	for _, run := range runs {
		hydratedRun, err := s.hydrateSkillRunPaperIDs(run, manifestByArtifactID[run.ArtifactID])
		if err != nil {
			return nil, err
		}
		switch run.TargetKind {
		case protocol.SkillTargetKindPaper:
			if _, ok := visiblePaperIDs[hydratedRun.TargetID]; ok {
				out = append(out, hydratedRun)
			}
		case protocol.SkillTargetKindComparison:
			if visiblePaperSetCovers(visiblePaperIDs, hydratedRun.PaperIDs) {
				out = append(out, hydratedRun)
			}
		}
	}
	return out, nil
}

func (s *Store) hydrateSkillRunPaperIDs(run protocol.SkillRunRecord, manifest protocol.ArtifactManifest) (protocol.SkillRunRecord, error) {
	run.PaperIDs = normalizedSkillPaperIDs(run.PaperIDs)
	if len(run.PaperIDs) > 0 {
		return run, nil
	}
	if run.TargetKind == protocol.SkillTargetKindPaper {
		run.PaperIDs = normalizedSkillPaperIDs([]string{run.TargetID})
		return run, nil
	}
	if ids := skillPaperIDsFromMetadata(manifest.Metadata); len(ids) > 0 {
		run.PaperIDs = ids
		return run, nil
	}
	ids, err := s.skillPaperIDsFromArtifactJSON(manifest)
	if err != nil {
		return protocol.SkillRunRecord{}, err
	}
	run.PaperIDs = ids
	return run, nil
}

func (s *Store) skillPaperIDsFromArtifactJSON(manifest protocol.ArtifactManifest) ([]string, error) {
	path := strings.TrimSpace(manifest.Paths["json"])
	if path == "" {
		return nil, nil
	}
	var payload struct {
		PaperIDs []string `json:"paper_ids"`
	}
	if err := s.loadJSON(path, &payload); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return normalizedSkillPaperIDs(payload.PaperIDs), nil
}

func skillPaperIDsFromMetadata(metadata map[string]interface{}) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["paper_ids"]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return normalizedSkillPaperIDs(value)
	case []interface{}:
		ids := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			ids = append(ids, text)
		}
		return normalizedSkillPaperIDs(ids)
	case string:
		return normalizedSkillPaperIDs([]string{value})
	default:
		return nil
	}
}

func visiblePaperSetCovers(visiblePaperIDs map[string]struct{}, paperIDs []string) bool {
	if len(paperIDs) == 0 {
		return false
	}
	for _, paperID := range paperIDs {
		if _, ok := visiblePaperIDs[paperID]; !ok {
			return false
		}
	}
	return true
}

func normalizedSkillPaperIDs(paperIDs []string) []string {
	if len(paperIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paperIDs))
	out := make([]string, 0, len(paperIDs))
	for _, paperID := range paperIDs {
		paperID = strings.TrimSpace(paperID)
		if paperID == "" {
			continue
		}
		if _, ok := seen[paperID]; ok {
			continue
		}
		seen[paperID] = struct{}{}
		out = append(out, paperID)
	}
	sort.Strings(out)
	return out
}

func filterVisibleSkillArtifacts(runs []protocol.SkillRunRecord, manifests []protocol.ArtifactManifest) []protocol.ArtifactManifest {
	if len(runs) == 0 || len(manifests) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(run.ArtifactID) == "" {
			continue
		}
		allowed[run.ArtifactID] = struct{}{}
	}
	out := make([]protocol.ArtifactManifest, 0, len(allowed))
	for _, manifest := range manifests {
		if _, ok := allowed[manifest.ArtifactID]; ok {
			out = append(out, manifest)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func buildWorkspaceResources(source protocol.PaperRef, artifacts []protocol.ArtifactManifest) []protocol.PaperResource {
	const (
		arxivBase    = "https://arxiv.org"
		alphaXivBase = "https://alphaxiv.org"
	)

	out := make([]protocol.PaperResource, 0, 7)
	seen := make(map[string]struct{}, 7)
	addResource := func(id, kind, title, uri string) {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			return
		}
		if _, ok := seen[uri]; ok {
			return
		}
		seen[uri] = struct{}{}
		out = append(out, protocol.PaperResource{
			ID:    id,
			Kind:  kind,
			Title: title,
			URI:   uri,
		})
	}

	sourceURI := source.URI
	if source.LocalPath != "" {
		sourceURI = source.LocalPath
	}
	addResource(source.PaperID+":source", "source", "Original source", sourceURI)

	if resolvedPaperID := strings.TrimSpace(source.ResolvedPaperID); resolvedPaperID != "" {
		addResource(source.PaperID+":arxiv_abs", "arxiv_abs", "arXiv abs", arxivBase+"/abs/"+resolvedPaperID)
		addResource(source.PaperID+":arxiv_pdf", "arxiv_pdf", "arXiv PDF", arxivBase+"/pdf/"+resolvedPaperID+".pdf")
		addResource(source.PaperID+":alphaxiv_overview", "alphaxiv_overview", "AlphaXiv overview", alphaXivBase+"/overview/"+resolvedPaperID)
		addResource(source.PaperID+":alphaxiv_full_text", "alphaxiv_full_text", "AlphaXiv full text", alphaXivBase+"/abs/"+resolvedPaperID)
	}

	for _, manifest := range artifacts {
		if manifest.Kind != "paper_digest" || manifest.ArtifactID != source.PaperID {
			continue
		}
		addResource(source.PaperID+":artifact_markdown", "artifact_markdown", "Digest markdown", manifest.Paths["markdown"])
		addResource(source.PaperID+":artifact_json", "artifact_json", "Digest JSON", manifest.Paths["json"])
	}

	return out
}
