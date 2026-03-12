package core

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"papersilm/internal/agent"
	"papersilm/internal/storage"
	"papersilm/pkg/protocol"
)

type EventSink interface {
	Emit(event protocol.StreamEvent) error
}

type Service struct {
	store *storage.Store
	agent *agent.Agent
	sink  EventSink
}

func New(store *storage.Store, ag *agent.Agent, sink EventSink) *Service {
	return &Service{
		store: store,
		agent: ag,
		sink:  sink,
	}
}

func (s *Service) NewSession(mode protocol.PermissionMode, lang, style string) (protocol.SessionMeta, error) {
	now := time.Now().UTC()
	meta := protocol.SessionMeta{
		SessionID:      newSessionID(),
		State:          protocol.SessionStateIdle,
		PermissionMode: mode,
		Language:       lang,
		Style:          style,
		CreatedAt:      now,
		UpdatedAt:      now,
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

func (s *Service) AttachSources(ctx context.Context, sessionID string, sources []string, replace bool) (protocol.SessionSnapshot, error) {
	return s.agent.AttachSources(ctx, s.store, s.sink, sessionID, sources, replace)
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
