package turnloop

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zzqDeco/papersilm/internal/runtime/input"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

var ErrRuntimeNotReady = errors.New("eino runtime turnloop is not wired yet")

type Handler func(context.Context, TurnEnvelope) (protocol.RunResult, error)

type Config struct {
	Handler Handler
	Clock   func() time.Time
}

type TurnEnvelope struct {
	TurnID    string
	SessionID string
	Items     []input.Item
	CreatedAt time.Time
}

type TurnLoop struct {
	dispatchMu sync.Mutex
	mu         sync.Mutex
	handler    Handler
	clock      func() time.Time
	buffer     map[string][]input.Item
}

func New(cfg Config) *TurnLoop {
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &TurnLoop{
		handler: cfg.Handler,
		clock:   clock,
		buffer:  map[string][]input.Item{},
	}
}

func (l *TurnLoop) Push(ctx context.Context, item input.Item) (protocol.RunResult, error) {
	if l.handler == nil {
		return protocol.RunResult{}, ErrRuntimeNotReady
	}
	l.dispatchMu.Lock()
	defer l.dispatchMu.Unlock()

	l.enqueue(item)
	var result protocol.RunResult
	for {
		envelope := l.drainNext(item.SessionID)
		if len(envelope.Items) == 0 {
			return result, nil
		}
		next, err := l.handler(ctx, envelope)
		if err != nil {
			l.restore(envelope)
			return protocol.RunResult{}, err
		}
		result = next
	}
}

func (l *TurnLoop) Buffered(sessionID string) []input.Item {
	l.mu.Lock()
	defer l.mu.Unlock()

	items := l.buffer[sessionID]
	return append([]input.Item(nil), items...)
}

func (l *TurnLoop) enqueue(item input.Item) {
	l.mu.Lock()
	defer l.mu.Unlock()

	item = l.normalizeItemLocked(item)
	l.buffer[item.SessionID] = append(l.buffer[item.SessionID], item)
}

func (l *TurnLoop) normalizeItemLocked(item input.Item) input.Item {
	now := l.clock()
	if item.ID == "" {
		item.ID = newID("input")
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	return item
}

func (l *TurnLoop) drain(sessionID string) TurnEnvelope {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.drainLocked(sessionID)
}

func (l *TurnLoop) drainNext(sessionID string) TurnEnvelope {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	items := l.buffer[sessionID]
	if len(items) == 0 {
		return TurnEnvelope{SessionID: sessionID, CreatedAt: now}
	}
	next := items[0]
	if len(items) == 1 {
		delete(l.buffer, sessionID)
	} else {
		l.buffer[sessionID] = append([]input.Item(nil), items[1:]...)
	}
	return TurnEnvelope{
		TurnID:    newID("turn"),
		SessionID: sessionID,
		Items:     []input.Item{next},
		CreatedAt: now,
	}
}

func (l *TurnLoop) drainLocked(sessionID string) TurnEnvelope {
	now := l.clock()
	items := append([]input.Item(nil), l.buffer[sessionID]...)
	delete(l.buffer, sessionID)
	return TurnEnvelope{
		TurnID:    newID("turn"),
		SessionID: sessionID,
		Items:     items,
		CreatedAt: now,
	}
}

func (l *TurnLoop) restore(envelope TurnEnvelope) {
	if len(envelope.Items) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	existing := append([]input.Item(nil), l.buffer[envelope.SessionID]...)
	restored := append([]input.Item(nil), envelope.Items...)
	l.buffer[envelope.SessionID] = append(restored, existing...)
}

func newID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
