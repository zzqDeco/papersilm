package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/storage"
	"github.com/zzqDeco/papersilm/pkg/core"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

type TUIOptions struct {
	Config         config.Config
	ContinueLatest bool
	ResumeID       string
	Mode           protocol.PermissionMode
	Lang           string
	Style          string
}

type tuiEventSink struct {
	ch chan protocol.StreamEvent
}

func newTUIEventSink(buffer int) *tuiEventSink {
	if buffer <= 0 {
		buffer = 256
	}
	return &tuiEventSink{ch: make(chan protocol.StreamEvent, buffer)}
}

func (s *tuiEventSink) Emit(event protocol.StreamEvent) error {
	s.ch <- event
	return nil
}

type tuiRuntimeManager struct {
	ctx   context.Context
	cfg   config.Config
	store *storage.Store
	svc   *core.Service
	sink  *tuiEventSink
}

func newTUIRuntimeManager(ctx context.Context, opts TUIOptions) (*tuiRuntimeManager, protocol.SessionSnapshot, error) {
	sink := newTUIEventSink(512)
	cfg, store, svc, err := buildServiceRuntime(ctx, opts.Config, sink)
	if err != nil {
		return nil, protocol.SessionSnapshot{}, err
	}
	snapshot, err := prepareSessionSnapshot(ctx, svc, store, opts.Mode, opts.Lang, opts.Style, opts.ContinueLatest, opts.ResumeID)
	if err != nil {
		return nil, protocol.SessionSnapshot{}, err
	}
	manager := &tuiRuntimeManager{
		ctx:   ctx,
		cfg:   cfg,
		store: store,
		svc:   svc,
		sink:  sink,
	}
	if changed, err := manager.ensureSessionRuntimeMeta(&snapshot); err != nil {
		return nil, protocol.SessionSnapshot{}, err
	} else if changed {
		snapshot, err = manager.store.Snapshot(snapshot.Meta.SessionID)
		if err != nil {
			return nil, protocol.SessionSnapshot{}, err
		}
	}
	return manager, snapshot, nil
}

func (r *tuiRuntimeManager) ensureSessionRuntimeMeta(snapshot *protocol.SessionSnapshot) (bool, error) {
	changed := false
	if strings.TrimSpace(snapshot.Meta.WorkspaceRoot) == "" {
		snapshot.Meta.WorkspaceRoot = r.store.WorkspaceRoot()
		changed = true
	}
	if strings.TrimSpace(snapshot.Meta.WorkspaceID) == "" && snapshot.Workspace != nil && strings.TrimSpace(snapshot.Workspace.WorkspaceID) != "" {
		snapshot.Meta.WorkspaceID = snapshot.Workspace.WorkspaceID
		changed = true
	}
	if strings.TrimSpace(snapshot.Meta.ProviderProfile) == "" {
		snapshot.Meta.ProviderProfile = r.cfg.ActiveProviderName()
		changed = true
	}
	if strings.TrimSpace(snapshot.Meta.Model) == "" {
		snapshot.Meta.Model = r.cfg.ActiveProviderConfig().Model
		changed = true
	}
	if !changed {
		return false, nil
	}
	snapshot.Meta.UpdatedAt = time.Now().UTC()
	return true, r.store.SaveMeta(snapshot.Meta)
}

func (r *tuiRuntimeManager) loadRecentEvents(sessionID string, limit int) ([]protocol.StreamEvent, error) {
	return r.store.LoadRecentEvents(sessionID, limit)
}

func (r *tuiRuntimeManager) loadTranscript(sessionID string) ([]protocol.TranscriptEntry, error) {
	return r.store.LoadTranscript(sessionID)
}

func (r *tuiRuntimeManager) drainPendingStartupEvents() {
	for {
		select {
		case <-r.sink.ch:
		default:
			return
		}
	}
}

func (r *tuiRuntimeManager) switchProviderModel(snapshot *protocol.SessionSnapshot, profile, model string) error {
	cfg := r.cfg
	if err := cfg.SetActiveProvider(profile); err != nil {
		return err
	}
	if err := cfg.SetActiveModel(model); err != nil {
		return err
	}
	if err := config.Save(config.ConfigPath(cfg.BaseDir), cfg); err != nil {
		return err
	}

	snapshot.Meta.ProviderProfile = profile
	snapshot.Meta.Model = strings.TrimSpace(model)
	snapshot.Meta.UpdatedAt = time.Now().UTC()
	if err := r.store.SaveMeta(snapshot.Meta); err != nil {
		return err
	}

	cfg, store, svc, err := buildServiceRuntime(r.ctx, cfg, r.sink)
	if err != nil {
		return err
	}
	r.cfg = cfg
	r.store = store
	r.svc = svc

	fresh, err := r.store.Snapshot(snapshot.Meta.SessionID)
	if err != nil {
		return err
	}
	*snapshot = fresh
	return nil
}

func (r *tuiRuntimeManager) profileNames() []string {
	return r.cfg.ProviderProfileNames()
}

func (r *tuiRuntimeManager) providerProfile(name string) (config.ProviderConfig, bool) {
	provider, ok := r.cfg.Providers[name]
	return provider, ok
}

func (r *tuiRuntimeManager) discoverModels(profile string) ([]string, error) {
	provider, ok := r.providerProfile(profile)
	if !ok {
		return nil, fmt.Errorf("provider profile not found: %s", profile)
	}
	switch provider.Provider {
	case config.ProviderOllama:
		return discoverOllamaModels(provider)
	default:
		return discoverOpenAICompatibleModels(provider)
	}
}

func providerProfileBlocked(provider config.ProviderConfig) (bool, string) {
	if provider.Provider == "" {
		return true, "missing provider"
	}
	switch provider.Provider {
	case config.ProviderOllama:
		if strings.TrimSpace(provider.BaseURL) == "" {
			return true, "missing base_url"
		}
	default:
		if strings.TrimSpace(provider.APIKey) == "" {
			return true, "missing api_key"
		}
	}
	return false, ""
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type ollamaModelsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func discoverOpenAICompatibleModels(provider config.ProviderConfig) ([]string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(provider.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discover models failed: %s", resp.Status)
	}
	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		models = append(models, item.ID)
	}
	sort.Strings(models)
	return uniqueStrings(models), nil
}

func discoverOllamaModels(provider config.ProviderConfig) ([]string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discover models failed: %s", resp.Status)
	}
	var payload ollamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		models = append(models, item.Name)
	}
	sort.Strings(models)
	return uniqueStrings(models), nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	var prev string
	for _, value := range values {
		if value == "" || value == prev {
			continue
		}
		out = append(out, value)
		prev = value
	}
	return out
}
