package turnloop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zzqDeco/papersilm/internal/runtime/input"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func TestPushBuffersWhenRuntimeNotReady(t *testing.T) {
	t.Parallel()

	loop := New(Config{})
	_, err := loop.Push(context.Background(), input.Item{
		SessionID: "sess_1",
		Kind:      input.KindUserMessage,
		Text:      "hello",
	})
	if !errors.Is(err, ErrRuntimeNotReady) {
		t.Fatalf("expected ErrRuntimeNotReady, got %v", err)
	}
	buffered := loop.Buffered("sess_1")
	if len(buffered) != 1 {
		t.Fatalf("expected buffered input, got %+v", buffered)
	}
	if buffered[0].ID == "" || buffered[0].CreatedAt.IsZero() {
		t.Fatalf("expected normalized input metadata, got %+v", buffered[0])
	}
}

func TestPushDrainsBufferedItemsIntoHandler(t *testing.T) {
	t.Parallel()

	var got TurnEnvelope
	loop := New(Config{
		Clock: func() time.Time { return time.Unix(123, 0).UTC() },
		Handler: func(_ context.Context, envelope TurnEnvelope) (protocol.RunResult, error) {
			got = envelope
			return protocol.RunResult{}, nil
		},
	})
	if _, err := loop.Push(context.Background(), input.Item{SessionID: "sess_1", Text: "hello"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got.SessionID != "sess_1" || len(got.Items) != 1 {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	if len(loop.Buffered("sess_1")) != 0 {
		t.Fatalf("expected buffer to drain")
	}
}
