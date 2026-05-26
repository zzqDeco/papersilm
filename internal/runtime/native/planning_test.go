package native

import (
	"errors"
	"os"
	"testing"
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
