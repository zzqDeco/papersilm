package core

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/agent"
	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type EventSink interface {
	Emit(event protocol.StreamEvent) error
}

type Service struct {
	cfg   config.Config
	store *storage.Store
	agent *agent.Agent
	sink  EventSink
}

func New(cfg config.Config, store *storage.Store, ag *agent.Agent, sink EventSink) *Service {
	return &Service{
		cfg:   cfg,
		store: store,
		agent: ag,
		sink:  sink,
	}
}

func (s *Service) NewSession(mode protocol.PermissionMode, lang, style string) (protocol.SessionMeta, error) {
	now := time.Now().UTC()
	workspaceRoot := strings.TrimSpace(s.store.WorkspaceRoot())
	workspaceID := protocol.DefaultWorkspaceID
	if summary, err := s.store.LoadWorkspaceSummary(); err == nil && summary != nil {
		workspaceRoot = strings.TrimSpace(summary.Root)
		if strings.TrimSpace(summary.WorkspaceID) != "" {
			workspaceID = summary.WorkspaceID
		}
	}
	meta := protocol.SessionMeta{
		SessionID:       newSessionID(),
		State:           protocol.SessionStateIdle,
		PermissionMode:  mode,
		WorkspaceRoot:   workspaceRoot,
		WorkspaceID:     workspaceID,
		ProviderProfile: s.cfg.ActiveProviderName(),
		Model:           s.cfg.ActiveProviderConfig().Model,
		Language:        lang,
		Style:           style,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.CreateSession(meta); err != nil {
		return meta, err
	}
	_ = s.emit(meta.SessionID, protocol.EventInit, "session created", meta)
	return meta, nil
}

func (s *Service) LoadSession(sessionID string) (protocol.SessionSnapshot, error) {
	snapshot, err := s.store.Snapshot(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	_ = s.emit(sessionID, protocol.EventSessionLoaded, "session loaded", snapshot.Meta)
	return snapshot, nil
}

func (s *Service) LatestSession() (protocol.SessionSnapshot, error) {
	sessionID, err := s.store.LatestSessionID()
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return s.LoadSession(sessionID)
}

func (s *Service) Execute(ctx context.Context, req protocol.ClientRequest) (protocol.RunResult, error) {
	if req.SessionID == "" {
		meta, err := s.NewSession(req.PermissionMode, req.Language, req.Style)
		if err != nil {
			return protocol.RunResult{}, err
		}
		req.SessionID = meta.SessionID
	}
	return s.agent.Execute(ctx, s.store, s.sink, req)
}

func (s *Service) RunPlanned(ctx context.Context, sessionID, lang, style string) (protocol.RunResult, error) {
	return s.agent.RunPlanned(ctx, s.store, s.sink, sessionID, lang, style)
}

func (s *Service) Approve(ctx context.Context, sessionID string, approved bool, comment string) (protocol.RunResult, error) {
	return s.agent.Approve(ctx, s.store, s.sink, sessionID, approved, comment)
}

func (s *Service) LoadTaskBoard(sessionID string) (*protocol.TaskBoard, error) {
	snapshot, err := s.store.Snapshot(sessionID)
	if err != nil {
		return nil, err
	}
	return snapshot.TaskBoard, nil
}

func (s *Service) RunTask(ctx context.Context, sessionID, taskID, lang, style string) (protocol.RunResult, error) {
	return s.agent.RunTask(ctx, s.store, s.sink, sessionID, taskID, lang, style)
}

func (s *Service) ApproveTask(ctx context.Context, sessionID, taskID string, approved bool, comment string) (protocol.RunResult, error) {
	return s.agent.ApproveTask(ctx, s.store, s.sink, sessionID, taskID, approved, comment)
}

func (s *Service) RejectTask(ctx context.Context, sessionID, taskID, comment string) (protocol.RunResult, error) {
	return s.agent.RejectTask(ctx, s.store, s.sink, sessionID, taskID, comment)
}

func (s *Service) DecidePermission(ctx context.Context, sessionID string, decision protocol.PermissionDecision) (protocol.RunResult, error) {
	return s.agent.DecidePermission(ctx, s.store, s.sink, sessionID, decision)
}

func (s *Service) ListSkills(sessionID string) ([]protocol.SkillDescriptor, error) {
	meta, err := s.store.LoadMeta(sessionID)
	if err != nil {
		return nil, err
	}
	return s.agent.ListSkills(meta.Language)
}

func (s *Service) RunSkill(ctx context.Context, sessionID, skillName, targetID string) (protocol.SkillRunResult, error) {
	return s.agent.RunSkill(ctx, s.store, s.sink, sessionID, skillName, targetID)
}

func (s *Service) LoadSkillRun(sessionID, runID string) (protocol.SkillRunRecord, error) {
	run, err := s.store.LoadSkillRun(sessionID, runID)
	if err != nil {
		return protocol.SkillRunRecord{}, err
	}
	if run == nil {
		return protocol.SkillRunRecord{}, fmt.Errorf("skill run not found: %s", runID)
	}
	return *run, nil
}

func (s *Service) AttachSources(ctx context.Context, sessionID string, sources []string, replace bool) (protocol.SessionSnapshot, error) {
	return s.agent.AttachSources(ctx, s.store, s.sink, sessionID, sources, replace)
}

func (s *Service) LoadWorkspaces(sessionID string) ([]protocol.PaperWorkspace, error) {
	snapshot, err := s.store.Snapshot(sessionID)
	if err != nil {
		return nil, err
	}
	return snapshot.Workspaces, nil
}

func (s *Service) AddWorkspaceNote(sessionID, paperID, body string) (protocol.SessionSnapshot, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return protocol.SessionSnapshot{}, fmt.Errorf("workspace note body is required")
	}

	snapshot, err := s.store.Snapshot(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	if !workspaceExists(snapshot.Workspaces, paperID) {
		return protocol.SessionSnapshot{}, fmt.Errorf("workspace not found: %s", paperID)
	}

	workspace, err := s.loadWorkspaceState(snapshot, paperID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}

	now := time.Now().UTC()
	workspace.Notes = append(workspace.Notes, protocol.PaperNote{
		ID:        fmt.Sprintf("note_%d", now.UnixNano()),
		Title:     deriveWorkspaceTitle(body),
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if workspace.CreatedAt.IsZero() {
		workspace.CreatedAt = now
	}
	workspace.UpdatedAt = now
	if err := s.store.SaveWorkspaceState(sessionID, workspace); err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return s.store.Snapshot(sessionID)
}

func (s *Service) AddWorkspaceAnnotation(sessionID, paperID string, anchor protocol.AnchorRef, body string) (protocol.SessionSnapshot, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return protocol.SessionSnapshot{}, fmt.Errorf("workspace annotation body is required")
	}
	anchor, err := normalizeWorkspaceAnchor(anchor)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}

	snapshot, err := s.store.Snapshot(sessionID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}
	if !workspaceExists(snapshot.Workspaces, paperID) {
		return protocol.SessionSnapshot{}, fmt.Errorf("workspace not found: %s", paperID)
	}

	workspace, err := s.loadWorkspaceState(snapshot, paperID)
	if err != nil {
		return protocol.SessionSnapshot{}, err
	}

	now := time.Now().UTC()
	workspace.Annotations = append(workspace.Annotations, protocol.PaperAnnotation{
		ID:        fmt.Sprintf("ann_%d", now.UnixNano()),
		Title:     deriveWorkspaceTitle(body),
		Body:      body,
		Anchor:    anchor,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if workspace.CreatedAt.IsZero() {
		workspace.CreatedAt = now
	}
	workspace.UpdatedAt = now
	if err := s.store.SaveWorkspaceState(sessionID, workspace); err != nil {
		return protocol.SessionSnapshot{}, err
	}
	return s.store.Snapshot(sessionID)
}

func (s *Service) emit(sessionID string, eventType protocol.StreamEventType, message string, payload interface{}) error {
	event := protocol.StreamEvent{
		Type:      eventType,
		SessionID: sessionID,
		Message:   message,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	if s.sink != nil {
		if err := s.sink.Emit(event); err != nil {
			return err
		}
	}
	return s.store.AppendEvent(sessionID, event)
}

func newSessionID() string {
	var b [10]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return "sess_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

func (s *Service) loadWorkspaceState(snapshot protocol.SessionSnapshot, paperID string) (protocol.PaperWorkspace, error) {
	workspace, err := s.store.LoadWorkspaceState(snapshot.Meta.SessionID, paperID)
	if err != nil {
		return protocol.PaperWorkspace{}, err
	}
	if workspace != nil {
		if workspace.CreatedAt.IsZero() {
			workspace.CreatedAt = snapshot.Meta.CreatedAt
		}
		if workspace.UpdatedAt.IsZero() {
			workspace.UpdatedAt = snapshot.Meta.UpdatedAt
		}
		if workspace.Notes == nil {
			workspace.Notes = []protocol.PaperNote{}
		}
		if workspace.Annotations == nil {
			workspace.Annotations = []protocol.PaperAnnotation{}
		}
		if workspace.Similar == nil {
			workspace.Similar = []protocol.SimilarPaperRef{}
		}
		return *workspace, nil
	}
	return protocol.PaperWorkspace{
		PaperID:     paperID,
		Notes:       []protocol.PaperNote{},
		Annotations: []protocol.PaperAnnotation{},
		Similar:     []protocol.SimilarPaperRef{},
		CreatedAt:   snapshot.Meta.CreatedAt,
		UpdatedAt:   snapshot.Meta.UpdatedAt,
	}, nil
}

func workspaceExists(workspaces []protocol.PaperWorkspace, paperID string) bool {
	for _, workspace := range workspaces {
		if workspace.PaperID == paperID {
			return true
		}
	}
	return false
}

func normalizeWorkspaceAnchor(anchor protocol.AnchorRef) (protocol.AnchorRef, error) {
	switch anchor.Kind {
	case protocol.AnchorKindPage:
		if anchor.Page <= 0 {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation page must be greater than 0")
		}
		anchor.Snippet = ""
		anchor.Section = ""
		return anchor, nil
	case protocol.AnchorKindSnippet:
		anchor.Snippet = strings.TrimSpace(anchor.Snippet)
		if anchor.Snippet == "" {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation snippet is required")
		}
		anchor.Page = 0
		anchor.Section = ""
		return anchor, nil
	case protocol.AnchorKindSection:
		anchor.Section = strings.TrimSpace(anchor.Section)
		if anchor.Section == "" {
			return protocol.AnchorRef{}, fmt.Errorf("workspace annotation section is required")
		}
		anchor.Page = 0
		anchor.Snippet = ""
		return anchor, nil
	default:
		return protocol.AnchorRef{}, fmt.Errorf("unsupported workspace anchor kind: %s", anchor.Kind)
	}
}

func deriveWorkspaceTitle(body string) string {
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if body == "" {
		return ""
	}
	for idx, r := range body {
		switch r {
		case '。', '！', '？', '!', '?', '\n', '\r':
			return truncateWorkspaceTitle(body[:idx])
		}
	}
	return truncateWorkspaceTitle(body)
}

func truncateWorkspaceTitle(in string) string {
	runes := []rune(strings.TrimSpace(in))
	if len(runes) <= 60 {
		return string(runes)
	}
	return string(runes[:60]) + "..."
}
