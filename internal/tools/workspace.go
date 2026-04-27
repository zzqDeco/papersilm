package tools

import (
	"context"
	"strings"

	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

func (r *Registry) LoadWorkspaceSummary(store *storage.Store) (*protocol.WorkspaceSummary, error) {
	return store.LoadWorkspaceSummary()
}

func (r *Registry) LoadWorkspaceFiles(store *storage.Store) ([]protocol.WorkspaceFile, error) {
	return store.LoadWorkspaceFiles()
}

func (r *Registry) WorkspacePaperCandidates(store *storage.Store) ([]protocol.WorkspaceFile, error) {
	return store.WorkspacePaperCandidates()
}

func (r *Registry) SearchWorkspace(_ context.Context, store *storage.Store, query string, limit int) ([]protocol.WorkspaceSearchHit, error) {
	return store.SearchWorkspace(query, limit)
}

func (r *Registry) ReadWorkspaceFile(store *storage.Store, path string) (string, error) {
	return store.ReadWorkspaceFile(path)
}

func (r *Registry) WriteWorkspaceFile(store *storage.Store, path, content string) error {
	return store.WriteWorkspaceFile(path, content)
}

func (r *Registry) ReplaceWorkspaceFile(store *storage.Store, path, oldValue, newValue string) error {
	return store.ReplaceWorkspaceFile(path, oldValue, newValue)
}

func (r *Registry) RunWorkspaceCommand(store *storage.Store, command string) (protocol.WorkspaceCommandRecord, error) {
	return store.RunWorkspaceCommand(strings.TrimSpace(command))
}
