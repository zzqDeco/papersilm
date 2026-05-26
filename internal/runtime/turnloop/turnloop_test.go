package turnloop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	if _, ok := loop.buffer["sess_1"]; ok {
		t.Fatalf("expected drained session key to be deleted")
	}
}

func TestPushRestoresDrainedItemsWhenHandlerFails(t *testing.T) {
	t.Parallel()

	handlerErr := errors.New("handler failed")
	loop := New(Config{
		Handler: func(_ context.Context, envelope TurnEnvelope) (protocol.RunResult, error) {
			if len(envelope.Items) != 1 {
				t.Fatalf("expected one item in envelope, got %+v", envelope.Items)
			}
			return protocol.RunResult{}, handlerErr
		},
	})
	_, err := loop.Push(context.Background(), input.Item{
		SessionID: "sess_1",
		Kind:      input.KindUserMessage,
		Text:      "retry me",
	})
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handler error, got %v", err)
	}
	buffered := loop.Buffered("sess_1")
	if len(buffered) != 1 || buffered[0].Text != "retry me" {
		t.Fatalf("expected failed turn input restored, got %+v", buffered)
	}
}

func TestConcurrentPushNeverDispatchesEmptyTurn(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	seen := make([]TurnEnvelope, 0, 32)
	loop := New(Config{
		Handler: func(_ context.Context, envelope TurnEnvelope) (protocol.RunResult, error) {
			mu.Lock()
			seen = append(seen, envelope)
			mu.Unlock()
			return protocol.RunResult{}, nil
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := loop.Push(context.Background(), input.Item{
				SessionID: "sess_1",
				Kind:      input.KindUserMessage,
				Text:      fmt.Sprintf("message %d", i),
			})
			if err != nil {
				t.Errorf("Push(%d): %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatalf("expected dispatched turns")
	}
	totalItems := 0
	for _, envelope := range seen {
		if len(envelope.Items) == 0 {
			t.Fatalf("empty turn dispatched: %+v", envelope)
		}
		totalItems += len(envelope.Items)
	}
	if totalItems != 32 {
		t.Fatalf("expected all inputs to dispatch once, got %d", totalItems)
	}
}

func TestConcurrentPushSerializesHandlerDispatch(t *testing.T) {
	t.Parallel()

	var active int32
	loop := New(Config{
		Handler: func(_ context.Context, envelope TurnEnvelope) (protocol.RunResult, error) {
			if len(envelope.Items) == 0 {
				t.Fatalf("empty turn dispatched")
			}
			if n := atomic.AddInt32(&active, 1); n != 1 {
				t.Fatalf("handler ran concurrently, active=%d", n)
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return protocol.RunResult{}, nil
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := loop.Push(context.Background(), input.Item{
				SessionID: "sess_1",
				Kind:      input.KindUserMessage,
				Text:      fmt.Sprintf("message %d", i),
			}); err != nil {
				t.Errorf("Push(%d): %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}
