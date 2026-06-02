# Security Policy

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

This project exposes SAP ADT — a privileged development interface — as an MCP server. Vulnerabilities here can affect SAP system integrity, ABAP source code, and transport management.

Report vulnerabilities privately by email to:

**konstantin.klein@hochfrequenz.de**

Please include:
- A description of the vulnerability and its impact.
- Steps to reproduce (tool call + arguments JSON if applicable).
- The affected SAP system type (ECC / S/4) if relevant.
- Any suggested fix.

We aim to respond within 5 business days and to publish a fix or mitigation within 90 days of the report.

## Scope

In scope:
- Command injection or ABAP injection via MCP tool parameters.
- Authentication bypass or credential exposure.
- Unintended data exfiltration via tool results.
- Lock/unlock bypass leading to lost-update vulnerabilities.

Out of scope:
- SAP system vulnerabilities not caused by this MCP server.
- Vulnerabilities requiring physical access to the SAP host.
- Denial-of-service against the SAP system via legitimate tool calls (rate limiting is the caller's responsibility).

## Supported versions

We only maintain the latest release on `main`. Older releases do not receive security fixes.
