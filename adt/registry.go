package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dachner/mcp-server-abap/config"
)

// ClientRegistry holds multiple named ADT clients and tracks which is active.
// It implements adt.Client by delegating all calls to the currently active client.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]Client
	configs map[string]config.SAPConfig
	active  string
}

// NewClientRegistry creates one Client per system in cfg, with cfg.DefaultSystem active.
func NewClientRegistry(cfg *config.Config) (*ClientRegistry, error) {
	clients := make(map[string]Client, len(cfg.Systems))
	for name, sysCfg := range cfg.Systems {
		clients[name] = NewClient(sysCfg)
	}
	return &ClientRegistry{
		clients: clients,
		configs: cfg.Systems,
		active:  cfg.DefaultSystem,
	}, nil
}

// Select switches the active system. Returns a display string including the system name and host.
// Takes a write lock — in-flight requests against the previous system complete normally.
func (r *ClientRegistry) Select(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[name]; !ok {
		names := make([]string, 0, len(r.clients))
		for n := range r.clients {
			names = append(names, n)
		}
		sort.Strings(names)
		return "", fmt.Errorf("unknown system %q, available: %s", name, strings.Join(names, ", "))
	}
	r.active = name
	return fmt.Sprintf("Active system: %s (%s)", name, r.configs[name].Host), nil
}

// ActiveName returns the name of the currently active system.
func (r *ClientRegistry) ActiveName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *ClientRegistry) activeClient() Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[r.active]
}

// --- adt.Client delegation (all 11 methods) ---

func (r *ClientRegistry) GetSource(ctx context.Context, objectURI string) (*SourceResult, error) {
	return r.activeClient().GetSource(ctx, objectURI)
}
func (r *ClientRegistry) SetSource(ctx context.Context, objectURI, source, etag string) error {
	return r.activeClient().SetSource(ctx, objectURI, source, etag)
}
func (r *ClientRegistry) ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error) {
	return r.activeClient().ActivateObject(ctx, objectURI)
}
func (r *ClientRegistry) SearchObjects(ctx context.Context, query, objectType string, maxResults int) ([]ObjectInfo, error) {
	return r.activeClient().SearchObjects(ctx, query, objectType, maxResults)
}
func (r *ClientRegistry) WhereUsed(ctx context.Context, objectURI string) ([]ObjectInfo, error) {
	return r.activeClient().WhereUsed(ctx, objectURI)
}
func (r *ClientRegistry) BrowsePackage(ctx context.Context, packageName string) ([]ObjectInfo, error) {
	return r.activeClient().BrowsePackage(ctx, packageName)
}
func (r *ClientRegistry) GetObjectInfo(ctx context.Context, objectURI string) (*ObjectInfo, error) {
	return r.activeClient().GetObjectInfo(ctx, objectURI)
}
func (r *ClientRegistry) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	return r.activeClient().SyntaxCheck(ctx, objectURI)
}
func (r *ClientRegistry) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	return r.activeClient().RunUnitTests(ctx, objectURI, timeoutSeconds)
}
func (r *ClientRegistry) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	return r.activeClient().GetTransportRequests(ctx, user, status)
}
func (r *ClientRegistry) AddToTransport(ctx context.Context, objectURI, transport string) error {
	return r.activeClient().AddToTransport(ctx, objectURI, transport)
}
