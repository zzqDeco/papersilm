package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/zzqDeco/papersilm/internal/agent"
	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/pipeline"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/internal/tools"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

var ErrTUIStartup = errors.New("tui startup failed")

func loadConfig() (config.Config, error) {
	return config.Load(config.ConfigPath(""))
}

func resolveWorkspaceStoreDir() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && strings.TrimSpace(resolved) != "" {
		root = resolved
	}
	return filepath.Join(root, ".papersilm"), nil
}

func buildServiceRuntime(ctx context.Context, cfg config.Config, sink core.EventSink) (config.Config, *storage.Store, *core.Service, error) {
	cfg.Normalize()
	workspaceStoreDir, err := resolveWorkspaceStoreDir()
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	store := storage.New(workspaceStoreDir)
	if err := store.Ensure(); err != nil {
		return config.Config{}, nil, nil, err
	}
	runtimeCfg := cfg
	runtimeCfg.BaseDir = workspaceStoreDir
	p := pipeline.New(runtimeCfg)
	registry := tools.New(p)
	ag := agent.New(registry, cfg)
	svc := core.New(cfg, store, ag, sink)
	_ = ctx
	return cfg, store, svc, nil
}

func buildRuntime(ctx context.Context, outputFormat string) (config.Config, *storage.Store, *core.Service, *OutputWriter, error) {
	cfg, err := loadConfig()
	if err != nil {
		return config.Config{}, nil, nil, nil, err
	}
	out := NewOutputWriter(os.Stdout, protocol.OutputFormat(outputFormat))
	cfg, store, svc, err := buildServiceRuntime(ctx, cfg, out)
	if err != nil {
		return config.Config{}, nil, nil, nil, err
	}
	return cfg, store, svc, out, nil
}

func prepareSessionSnapshot(
	ctx context.Context,
	svc *core.Service,
	store *storage.Store,
	mode protocol.PermissionMode,
	lang, style string,
	continueLatest bool,
	resumeID string,
) (protocol.SessionSnapshot, error) {
	var (
		snapshot protocol.SessionSnapshot
		err      error
	)
	switch {
	case resumeID != "":
		snapshot, err = svc.LoadSession(resumeID)
		if err != nil {
			return protocol.SessionSnapshot{}, err
		}
	case continueLatest:
		snapshot, err = svc.LatestSession()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return protocol.SessionSnapshot{}, err
		}
	}
	if snapshot.Meta.SessionID == "" {
		meta, err := svc.NewSession(mode, lang, style)
		if err != nil {
			return protocol.SessionSnapshot{}, err
		}
		snapshot, err = store.Snapshot(meta.SessionID)
		if err != nil {
			return protocol.SessionSnapshot{}, err
		}
	}
	_ = ctx
	return snapshot, nil
}

func shouldUseTUI(outputFormat protocol.OutputFormat, printTask string) bool {
	return shouldUseTUIWithTTY(outputFormat, printTask, os.Getenv("TERM"), term.IsTerminal(int(os.Stdin.Fd())), term.IsTerminal(int(os.Stdout.Fd())))
}

func shouldUseTUIWithTTY(outputFormat protocol.OutputFormat, printTask, termValue string, stdinTTY, stdoutTTY bool) bool {
	if strings.TrimSpace(printTask) != "" {
		return false
	}
	if outputFormat != protocol.OutputFormatText {
		return false
	}
	if termValue == "dumb" {
		return false
	}
	return stdinTTY && stdoutTTY
}
