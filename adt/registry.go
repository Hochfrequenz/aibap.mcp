package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Hochfrequenz/mcp-server-abap/auth"
	"github.com/Hochfrequenz/mcp-server-abap/config"
)

// ClientRegistry holds multiple named ADT clients and tracks which is active.
// It implements adt.Client by delegating all calls to the currently active client.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]Client
	configs map[string]config.SAPSystem
	active  string
}

// NewClientRegistry creates one Client per system in cfg, with cfg.DefaultSystem active.
// For OAuth2 systems it loads the stored token and sets up automatic refresh.
func NewClientRegistry(cfg *config.AppConfig) (*ClientRegistry, error) {
	clients := make(map[string]Client, len(cfg.Systems))
	for name, sysCfg := range cfg.Systems {
		if sysCfg.IsOAuth2() {
			store := auth.NewTokenStore(auth.DefaultTokenPath())
			tokenData, err := store.TokenForSystem(name)
			if err != nil {
				return nil, fmt.Errorf("system %q requires OAuth2 login. Run: mcp-server-abap login %s", name, name)
			}
			systemName := name   // capture for closure
			sysCfgCopy := sysCfg // capture for closure
			td := tokenData      // mutable copy for closure
			onRefresh := func(currentToken string) (string, error) {
				newToken, err := auth.RefreshToken(
					sysCfgCopy.Host,
					config.EffectiveOAuth2ClientID(sysCfgCopy),
					td.RefreshToken,
					sysCfgCopy.TLSSkipVerify,
				)
				if err != nil {
					return "", fmt.Errorf("token refresh failed for %q: %w. Run: mcp-server-abap login %s", systemName, err, systemName)
				}
				// Save refreshed token
				_ = store.Save(systemName, newToken)
				td = newToken // update closure's copy
				return newToken.AccessToken, nil
			}
			clients[name] = NewClientWithToken(sysCfg, tokenData.AccessToken, onRefresh)
		} else {
			clients[name] = NewClient(sysCfg)
		}
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

// --- adt.Client delegation ---

func (r *ClientRegistry) GetSource(ctx context.Context, objectURI string) (*SourceResult, error) {
	return r.activeClient().GetSource(ctx, objectURI)
}
func (r *ClientRegistry) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	return r.activeClient().SetSource(ctx, objectURI, source, lockHandle, transport, etag)
}
func (r *ClientRegistry) ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error) {
	return r.activeClient().ActivateObjects(ctx, objectURIs)
}
func (r *ClientRegistry) GetInactiveObjects(ctx context.Context) ([]ObjectInfo, error) {
	return r.activeClient().GetInactiveObjects(ctx)
}
func (r *ClientRegistry) GetABAPDoc(ctx context.Context, keyword string) (string, error) {
	return r.activeClient().GetABAPDoc(ctx, keyword)
}
func (r *ClientRegistry) GetTextElements(ctx context.Context, objectURI string) (*TextElements, error) {
	return r.activeClient().GetTextElements(ctx, objectURI)
}
func (r *ClientRegistry) GetMessageClass(ctx context.Context, messageClassName string) (*MessageClassInfo, error) {
	return r.activeClient().GetMessageClass(ctx, messageClassName)
}
func (r *ClientRegistry) SearchMessages(ctx context.Context, query string, maxResults int) ([]MessageSearchResult, error) {
	return r.activeClient().SearchMessages(ctx, query, maxResults)
}
func (r *ClientRegistry) SetMessages(ctx context.Context, messageClassName, etag string, messages []Message) error {
	return r.activeClient().SetMessages(ctx, messageClassName, etag, messages)
}
func (r *ClientRegistry) NavigateToDefinition(ctx context.Context, sourceURI string) (string, error) {
	return r.activeClient().NavigateToDefinition(ctx, sourceURI)
}
func (r *ClientRegistry) GetTableFields(ctx context.Context, tableName string) ([]FieldInfo, error) {
	return r.activeClient().GetTableFields(ctx, tableName)
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
func (r *ClientRegistry) BatchSyntaxCheck(ctx context.Context, objectURIs []string, workers int) []ObjectSyntaxResult {
	return r.activeClient().BatchSyntaxCheck(ctx, objectURIs, workers)
}
func (r *ClientRegistry) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	return r.activeClient().RunUnitTests(ctx, objectURI, timeoutSeconds)
}
func (r *ClientRegistry) CreateTransport(ctx context.Context, category, target, description, devClass string) (string, error) {
	return r.activeClient().CreateTransport(ctx, category, target, description, devClass)
}
func (r *ClientRegistry) ReleaseTransport(ctx context.Context, transportNumber string) error {
	return r.activeClient().ReleaseTransport(ctx, transportNumber)
}
func (r *ClientRegistry) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	return r.activeClient().GetTransportRequests(ctx, user, status)
}
func (r *ClientRegistry) Rename(ctx context.Context, sourceURI, newName, transport string) (*RenameResult, error) {
	return r.activeClient().Rename(ctx, sourceURI, newName, transport)
}
func (r *ClientRegistry) GetVersionHistory(ctx context.Context, objectURI string) ([]VersionInfo, error) {
	return r.activeClient().GetVersionHistory(ctx, objectURI)
}
func (r *ClientRegistry) GetVersionSource(ctx context.Context, contentURI string) (string, error) {
	return r.activeClient().GetVersionSource(ctx, contentURI)
}
func (r *ClientRegistry) DiffActiveInactive(ctx context.Context, objectURI string) (*DiffResult, error) {
	return r.activeClient().DiffActiveInactive(ctx, objectURI)
}
func (r *ClientRegistry) CheckTransport(ctx context.Context, pgmID, object, objectName string) (*TransportCheckResult, error) {
	return r.activeClient().CheckTransport(ctx, pgmID, object, objectName)
}
func (r *ClientRegistry) AddToTransport(ctx context.Context, objectURI, transport string) error {
	return r.activeClient().AddToTransport(ctx, objectURI, transport)
}
func (r *ClientRegistry) LockObject(ctx context.Context, objectURI string) (string, error) {
	return r.activeClient().LockObject(ctx, objectURI)
}
func (r *ClientRegistry) UnlockObject(ctx context.Context, objectURI, lockHandle string) error {
	return r.activeClient().UnlockObject(ctx, objectURI, lockHandle)
}
func (r *ClientRegistry) PrettyPrint(ctx context.Context, source string) (string, error) {
	return r.activeClient().PrettyPrint(ctx, source)
}
func (r *ClientRegistry) CreateObject(ctx context.Context, objectType, name, packageName, description, transport string) error {
	return r.activeClient().CreateObject(ctx, objectType, name, packageName, description, transport)
}
func (r *ClientRegistry) CreateFunctionModule(ctx context.Context, groupName, moduleName, description, packageName, transport string) error {
	return r.activeClient().CreateFunctionModule(ctx, groupName, moduleName, description, packageName, transport)
}
func (r *ClientRegistry) CreatePackage(ctx context.Context, name, description, responsible, softwareComponent, transportLayer, transport string) error {
	return r.activeClient().CreatePackage(ctx, name, description, responsible, softwareComponent, transportLayer, transport)
}
func (r *ClientRegistry) DeleteObject(ctx context.Context, objectURI, lockHandle, transport string) error {
	return r.activeClient().DeleteObject(ctx, objectURI, lockHandle, transport)
}
func (r *ClientRegistry) GetCompletions(ctx context.Context, objectURI, source string, line, column int) ([]CompletionItem, error) {
	return r.activeClient().GetCompletions(ctx, objectURI, source, line, column)
}
func (r *ClientRegistry) ExportPackage(ctx context.Context, packageName string) ([]byte, error) {
	return r.activeClient().ExportPackage(ctx, packageName)
}
func (r *ClientRegistry) GetATCCustomizing(ctx context.Context) (*ATCCustomizingResult, error) {
	return r.activeClient().GetATCCustomizing(ctx)
}
func (r *ClientRegistry) RunATCCheck(ctx context.Context, objectURIs []string, checkVariant string) (*ATCResult, error) {
	return r.activeClient().RunATCCheck(ctx, objectURIs, checkVariant)
}
func (r *ClientRegistry) RunQuery(ctx context.Context, sql string, maxRows int) (*QueryResult, error) {
	return r.activeClient().RunQuery(ctx, sql, maxRows)
}
func (r *ClientRegistry) GetEnhancementSpot(ctx context.Context, spotName string) (*EnhancementSpotInfo, error) {
	return r.activeClient().GetEnhancementSpot(ctx, spotName)
}
func (r *ClientRegistry) GetEnhancementImplementation(ctx context.Context, implName string) (*BAdIImplementationInfo, error) {
	return r.activeClient().GetEnhancementImplementation(ctx, implName)
}
func (r *ClientRegistry) SystemInfo() (host, client string) {
	return r.activeClient().SystemInfo()
}
func (r *ClientRegistry) Logout(ctx context.Context) error {
	return r.activeClient().Logout(ctx)
}
