![Unittest status badge](https://github.com/Hochfrequenz/mcp-server-abap/workflows/Unittests/badge.svg)
![Coverage status badge](https://github.com/Hochfrequenz/mcp-server-abap/workflows/coverage/badge.svg)
![Linter status badge](https://github.com/Hochfrequenz/mcp-server-abap/workflows/golangci-lint/badge.svg)

# mcp-server-abap

A community-built [MCP (Model Context Protocol)](https://modelcontextprotocol.io) server that lets AI assistants like Claude read, write, and manage ABAP source code on SAP systems — directly from the editor.

> **Community project.** This project is not affiliated with, endorsed by, or supported by SAP SE. SAP and ABAP are trademarks of SAP SE.

---

## How it works

The server connects to your SAP system via the **SAP ADT (ABAP Development Tools) REST API** — the same HTTP API that ABAP Development Tools for Eclipse uses under the hood. No SAP GUI, no RFC, no additional middleware required.

```
Claude / AI assistant
        │  MCP (stdio)
        ▼
mcp-server-abap
        │  HTTP + Basic Auth + CSRF
        ▼
SAP ADT REST API  (/sap/bc/adt/...)
        │
        ▼
    SAP System
```

## Available tools

| Tool | Description |
|------|-------------|
| `get_source` | Read ABAP source code of any object |
| `set_source` | Write ABAP source code (requires ETag and lock handle) |
| `lock_object` | Lock an object for editing, returns a lock handle |
| `unlock_object` | Unlock a previously locked object |
| `activate_object` | Activate an ABAP object |
| `create_object` | Create a new ABAP object (program, class, interface) |
| `delete_object` | Delete an ABAP object |
| `search_objects` | Search for objects by name pattern and type |
| `where_used` | Find all usages of an object |
| `browse_package` | List contents of a package |
| `get_object_info` | Get object metadata (type, package, description) |
| `syntax_check` | Run a syntax check |
| `run_unit_tests` | Run ABAP Unit Tests |
| `pretty_print` | Format ABAP source code using SAP Pretty Printer |
| `get_completions` | Get code completion proposals at a cursor position |
| `get_transport_requests` | List open or released transport requests |
| `add_to_transport` | Assign an object to a transport request |
| `select_system` | Switch the active SAP system |

## Requirements

- SAP NetWeaver 7.40+ with ADT services active (transaction SICF: `/sap/bc/adt`)
- A user with developer authorizations (`S_ADT_RES`, `S_DEVELOP`)
- Go 1.26+ (to build from source)

## Installation

### Download binary

Download the latest release for your platform from the [releases page](https://github.com/Hochfrequenz/mcp-server-abap/releases).

### Build from source

```bash
git clone https://github.com/Hochfrequenz/mcp-server-abap.git
cd mcp-server-abap
go build -o mcp-server-abap .
```

## Configuration

Copy the example config and fill in your SAP system details:

```bash
cp config.yaml.example config.yaml
```

```yaml
default_system: dev

systems:
  dev:
    host: "https://your-dev-system:8000"
    client: "100"
    user: "YOUR_USER"
    password: "YOUR_PASSWORD"
    tls_skip_verify: false
  prod:
    host: "https://your-prod-system:8000"
    client: "200"
    user: "YOUR_USER"
    password: "YOUR_PASSWORD"
```

Alternatively, configure via environment variables:

| Variable | Description |
|----------|-------------|
| `SAP_CONFIG_FILE` | Path to config.yaml (default: `./config.yaml`) |

## Usage with Claude

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "abap": {
      "command": "/path/to/mcp-server-abap",
      "args": [],
      "env": {
        "SAP_CONFIG_FILE": "/path/to/config.yaml"
      }
    }
  }
}
```

### Claude Code (CLI)

Add to your Claude Code MCP settings or run directly:

```bash
SAP_CONFIG_FILE=/path/to/config.yaml mcp-server-abap
```

## Example workflow

Once connected, Claude can:

```
You: Show me the source of class ZCL_MY_SERVICE
Claude: [calls get_source] Here's the source...

You: Fix the bug in method GET_DATA and activate the class
Claude: [calls lock_object, set_source, activate_object, unlock_object] Done. Activation succeeded.

You: Run the unit tests for this class
Claude: [calls run_unit_tests] 5 tests passed, 0 failed.
```

## Contributing

Issues and pull requests welcome. This is a community project — if your SAP system exposes additional ADT endpoints you'd like to see supported, open an issue.

## License

MIT
