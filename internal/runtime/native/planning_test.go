package native

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type readErrorStore struct {
	err error
}

func (s readErrorStore) ReadWorkspaceFile(string) (string, error) {
	return "", s.err
}

func TestReadWorkspaceFileForEditStoreOnlyTreatsMissingAsAbsent(t *testing.T) {
	t.Parallel()

	content, existed, err := readWorkspaceFileForEditStore(readErrorStore{err: os.ErrNotExist}, "missing.md")
	if err != nil || existed || content != "" {
		t.Fatalf("missing file should be absent without error, content=%q existed=%v err=%v", content, existed, err)
	}

	permissionErr := errors.New("permission denied")
	_, _, err = readWorkspaceFileForEditStore(readErrorStore{err: permissionErr}, "secret.md")
	if !errors.Is(err, permissionErr) {
		t.Fatalf("expected non-missing read error to propagate, got %v", err)
	}
}

func TestExtractBacktickCommandPrefersRunnableCommand(t *testing.T) {
	t.Parallel()

	got := extractBacktickCommand("inspect `README.md` then run `go test ./...`")
	if got != "go test ./..." {
		t.Fatalf("expected runnable command, got %q", got)
	}

	got = extractBacktickCommand("run `printf %s hello`")
	if got != "printf %s hello" {
		t.Fatalf("single command segment should be preserved, got %q", got)
	}
}

func TestNextAssistantCheckpointIDIsInvocationScoped(t *testing.T) {
	t.Parallel()

	first := nextAssistantCheckpointID("edit_1")
	second := nextAssistantCheckpointID("edit_1")
	if first == second {
		t.Fatalf("checkpoint IDs should be unique per invocation, got %q", first)
	}
	if !strings.HasPrefix(first, "turn_edit_1_") || !strings.HasPrefix(second, "turn_edit_1_") {
		t.Fatalf("checkpoint IDs should preserve turn prefix, got %q and %q", first, second)
	}
}

func TestExecuteStepCommandNonZeroExitReturnsWorkspaceResult(t *testing.T) {
	t.Parallel()

	rt, store, _, _ := newNativeE2ERuntime(t)
	sessionID := newNativeE2ESession(t, store, protocol.PermissionModeAuto)
	step := protocol.PlanStep{
		ID:   "cmd_fail",
		Tool: string(protocol.NodeKindWorkspaceCommand),
		Goal: "run `printf stdout; printf stderr >&2; exit 7`",
	}
	out, err := rt.executeStep(context.Background(), sessionID, "run failing command", step, protocol.PermissionModeAuto)
	if err != nil {
		t.Fatalf("executeStep should not fail runtime for command exit code: %v", err)
	}
	text, _ := out.Data["response"].(string)
	for _, want := range []string{"command exited 7", "stdout", "stderr"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in command output, got %q", want, text)
		}
	}
}

func TestEditPermissionRequestUsesReplacePreviewForLocalizedEdit(t *testing.T) {
	t.Parallel()

	_, store, _, _ := newNativeE2ERuntime(t)
	if err := store.WriteWorkspaceFile("README.md", "hello typo world\n"); err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	request, err := editPermissionRequest(store, "sess_test", "plan_test", "edit_1", "update `README.md` replace `typo` with `type`", "README.md")
	if err != nil {
		t.Fatalf("editPermissionRequest: %v", err)
	}
	if request.Preview.OldText != "typo" || request.Preview.NewText != "type" {
		t.Fatalf("expected localized replace preview, got %+v", request.Preview)
	}
	if !strings.Contains(request.Preview.Diff, "-hello typo world") || !strings.Contains(request.Preview.Diff, "+hello type world") {
		t.Fatalf("expected replace diff, got:\n%s", request.Preview.Diff)
	}
	if request.PlanID != "plan_test" || request.NodeID != "edit_1" {
		t.Fatalf("expected native plan/node context to be preserved, got %+v", request)
	}
}
