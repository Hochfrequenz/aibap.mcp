package adt

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// ClientRegistry holds multiple named ADT clients and tracks which is active.
// It implements adt.Client by delegating all calls to the currently active client.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]Client
	active  string
}

// NewClientRegistry creates a registry over the given clients with defaultSystem
// active. It returns an error if clients is empty or if defaultSystem is not
// present in clients.
func NewClientRegistry(clients map[string]Client, defaultSystem string) (*ClientRegistry, error) {
	if len(clients) == 0 {
		return nil, fmt.Errorf("NewClientRegistry: no clients provided")
	}
	if _, ok := clients[defaultSystem]; !ok {
		return nil, fmt.Errorf("NewClientRegistry: default system %q not in clients (have: %s)", defaultSystem, strings.Join(slices.Sorted(maps.Keys(clients)), ", "))
	}
	return &ClientRegistry{
		clients: clients,
		active:  defaultSystem,
	}, nil
}

// Select switches the active system. Returns a display string including the system name and host.
// Takes a write lock — in-flight requests against the previous system complete normally.
func (r *ClientRegistry) Select(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	client, ok := r.clients[name]
	if !ok {
		return "", fmt.Errorf("unknown system %q, available: %s", name, strings.Join(slices.Sorted(maps.Keys(r.clients)), ", "))
	}
	r.active = name
	host, _ := client.SystemInfo()
	return fmt.Sprintf("Active system: %s (%s)", name, host), nil
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
func (r *ClientRegistry) GetClassDefinition(ctx context.Context, objectURI string) (*SourceResult, error) {
	return r.activeClient().GetClassDefinition(ctx, objectURI)
}
func (r *ClientRegistry) SetSource(ctx context.Context, objectURI, source, lockHandle, transport, etag string) (string, error) {
	return r.activeClient().SetSource(ctx, objectURI, source, lockHandle, transport, etag)
}
func (r *ClientRegistry) GetIncludeSource(ctx context.Context, objectURI, include string) (*SourceResult, error) {
	return r.activeClient().GetIncludeSource(ctx, objectURI, include)
}
func (r *ClientRegistry) SetIncludeSource(ctx context.Context, objectURI, include, source, lockHandle, transport, etag string) (string, error) {
	return r.activeClient().SetIncludeSource(ctx, objectURI, include, source, lockHandle, transport, etag)
}
func (r *ClientRegistry) CreateTestInclude(ctx context.Context, objectURI, lockHandle, transport string) error {
	return r.activeClient().CreateTestInclude(ctx, objectURI, lockHandle, transport)
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
func (r *ClientRegistry) BatchSyntaxCheck(ctx context.Context, objectURIs []string) []ObjectSyntaxResult {
	return r.activeClient().BatchSyntaxCheck(ctx, objectURIs)
}
func (r *ClientRegistry) RunUnitTests(ctx context.Context, objectURI string, timeoutSeconds int) (*TestResult, error) {
	return r.activeClient().RunUnitTests(ctx, objectURI, timeoutSeconds)
}
func (r *ClientRegistry) CreateTransport(ctx context.Context, category, target, description, devClass string) (string, error) {
	return r.activeClient().CreateTransport(ctx, category, target, description, devClass)
}
func (r *ClientRegistry) CreateTransportTask(ctx context.Context, parentTransport, owner, description string) (string, error) {
	return r.activeClient().CreateTransportTask(ctx, parentTransport, owner, description)
}
func (r *ClientRegistry) DeleteTransport(ctx context.Context, transportNumber string) error {
	return r.activeClient().DeleteTransport(ctx, transportNumber)
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
func (r *ClientRegistry) RemoveFromTransport(ctx context.Context, taskNumber, parentTransport, pgmID, objectType, objectName, wbType, position string) error {
	return r.activeClient().RemoveFromTransport(ctx, taskNumber, parentTransport, pgmID, objectType, objectName, wbType, position)
}
func (r *ClientRegistry) ReleaseTransportWithTasks(ctx context.Context, transportNumber string) error {
	return r.activeClient().ReleaseTransportWithTasks(ctx, transportNumber)
}
func (r *ClientRegistry) GetTransportInfo(ctx context.Context, transportNumber string) (*TransportRequest, error) {
	return r.activeClient().GetTransportInfo(ctx, transportNumber)
}
func (r *ClientRegistry) GetTransportObjects(ctx context.Context, transportNumber string) ([]TransportObject, error) {
	return r.activeClient().GetTransportObjects(ctx, transportNumber)
}
func (r *ClientRegistry) GetTransportTasks(ctx context.Context, transportNumber string) ([]string, error) {
	return r.activeClient().GetTransportTasks(ctx, transportNumber)
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
func (r *ClientRegistry) SetEnhancementImplementation(ctx context.Context, implName, xmlBody, lockHandle, transport, etag string) error {
	return r.activeClient().SetEnhancementImplementation(ctx, implName, xmlBody, lockHandle, transport, etag)
}
func (r *ClientRegistry) ListShortDumps(ctx context.Context, from, to, user string) ([]ShortDumpHeader, error) {
	return r.activeClient().ListShortDumps(ctx, from, to, user)
}
func (r *ClientRegistry) GetShortDumps(ctx context.Context, from, to, user string) ([]ShortDump, error) {
	return r.activeClient().GetShortDumps(ctx, from, to, user)
}
func (r *ClientRegistry) SystemInfo() (host, client string) {
	return r.activeClient().SystemInfo()
}
func (r *ClientRegistry) Logout(ctx context.Context) error {
	return r.activeClient().Logout(ctx)
}

// LogoutAll calls Logout on every registered client to end stateful SAP sessions
// and release ENQUEUE locks. Errors are collected but do not stop other logouts.
func (r *ClientRegistry) LogoutAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var firstErr error
	for _, c := range r.clients {
		if err := c.Logout(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
