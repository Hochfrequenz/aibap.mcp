# SAP ADT Client for Go

A Go library for the **SAP ADT (ABAP Development Tools) REST API** — the same HTTP API that Eclipse ADT uses under the hood.

Read, write, and manage ABAP source code on SAP systems programmatically. No SAP GUI, no RFC, no additional middleware required.

## Features

- **Source code** — read, write, patch, pretty-print ABAP source; class includes and definitions
- **Objects and packages** — search, create, delete, rename objects; browse packages; DDIC field metadata
- **Locking and activation** — pessimistic locking with auto-lock support; single and batch activation
- **Testing and quality** — syntax check, ABAP Unit tests, ATC static analysis
- **Transport management** — create, release, delete transports; add/remove objects; rollback
- **Version history** — list versions, read historical source, diff active vs inactive
- **Code intelligence** — code completion, navigate to definition, ABAP keyword docs
- **Messages and texts** — read/write message classes, text symbols, selection texts
- **Enhancements** — read/write BAdI enhancement spots and implementations
- **Debugging** — full debug lifecycle via stateful HTTP sessions
- **Data export** — package export (abapGit ZIP), customizing tables to SQLite/JSON
- **SQL queries** — execute SELECT queries on SAP database tables

## Installation

```bash
go get github.com/Hochfrequenz/mcp-server-abap/adt
```

## Quick start

```go
package main

import (
    "context"
    "fmt"

    sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
    "github.com/Hochfrequenz/mcp-server-abap/adt"
)

func main() {
    client := adt.NewClient(sapmcpconfig.SAPSystem{
        Host:     "https://your-sap-system:8000",
        User:     "YOUR_USER",
        Password: "YOUR_PASSWORD",
        Client:   "100",
    })

    ctx := context.Background()

    // Read source code
    result, err := client.GetSource(ctx, "/sap/bc/adt/programs/programs/ZMY_REPORT")
    if err != nil {
        panic(err)
    }
    fmt.Println(result.Source)

    // Search for objects
    objects, err := client.SearchObjects(ctx, "ZCL_MY*", "CLAS/OC", 10)
    if err != nil {
        panic(err)
    }
    for _, obj := range objects {
        fmt.Printf("%s (%s)\n", obj.Name, obj.Type)
    }
}
```

## Client interfaces

The `Client` interface is composed of focused sub-interfaces, so you can depend on only what you need:

| Interface | Methods |
|-----------|---------|
| `SourceClient` | `GetSource`, `SetSource`, `GetClassDefinition`, `GetIncludeSource`, `SetIncludeSource`, `CreateTestInclude`, `PrettyPrint`, `GetCompletions` |
| `ObjectClient` | `CreateObject`, `CreateFunctionModule`, `CreatePackage`, `DeleteObject`, `ActivateObjects`, `GetInactiveObjects` |
| `LockClient` | `LockObject`, `UnlockObject` |
| `SearchClient` | `SearchObjects`, `WhereUsed`, `BrowsePackage`, `GetObjectInfo` |
| `QualityClient` | `SyntaxCheck`, `BatchSyntaxCheck`, `RunUnitTests`, `RunATCCheck`, `GetATCCustomizing` |
| `TransportClient` | `CreateTransport`, `ReleaseTransport`, `GetTransportRequests`, `AddToTransport`, `RemoveFromTransport`, ... |
| `VersionClient` | `GetVersionHistory`, `GetVersionSource`, `DiffActiveInactive` |
| `DocuClient` | `GetABAPDoc`, `GetTextElements`, `GetMessageClass`, `SearchMessages`, `SetMessages` |
| `EnhancementClient` | `GetEnhancementSpot`, `GetEnhancementImplementation`, `SetEnhancementImplementation` |
| `QueryClient` | `RunQuery` |
| `ExportClient` | `ExportPackage` |
| `DDICClient` | `GetTableFields` |
| `DumpClient` | `ListShortDumps`, `GetShortDumps` |
| `NavigationClient` | `NavigateToDefinition` |
| `RefactoringClient` | `Rename` |
| `SystemClient` | `SystemInfo`, `Logout` |

## Multi-system support

Use `ClientRegistry` to manage multiple SAP systems:

```go
cfg, _ := sapmcpconfig.Load("~/.config/sap-mcp/systems.json")
registry, _ := adt.NewClientRegistry(cfg, "my-app")

// Switch systems at runtime
registry.Select("prod")
result, _ := registry.GetSource(ctx, objectURI)
```

## Sub-packages

| Package | Purpose |
|---------|---------|
| `adt/adtxml` | XML serialization helpers for ADT response formats |
| `adt/custexport` | Customizing table export to SQLite and JSON |

## Requirements

- SAP NetWeaver 7.40+ with ADT services active (SICF: `/sap/bc/adt`)
- A user with developer authorizations (`S_ADT_RES`, `S_DEVELOP`)
- Go 1.26+

## Testing

```bash
# Unit tests (no SAP system needed)
go test ./adt/...

# Integration tests (requires real SAP system)
go test -tags integration ./adt/... -run TestSpecificFunc
```

Integration tests require the [Z_ADT_MCP_TEST](https://github.com/Hochfrequenz/Z_ADT_MCP_TEST) package on the target SAP system.

## License

MIT — see [LICENSE](LICENSE).
