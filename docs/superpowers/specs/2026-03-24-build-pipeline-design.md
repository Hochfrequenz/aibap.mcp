# Build Pipeline: Binaries, Docker, Smoke Tests

**Date:** 2026-03-24

## Problem

The existing release pipeline is broken (wrong org name `dachner`, Go 1.22) and there's no Docker image. Users need easy access to pre-built binaries and a Docker option.

## Goals

1. Pre-built binaries for linux/darwin/windows (amd64+arm64) via GitHub Releases
2. Docker image on GHCR (`ghcr.io/hochfrequenz/mcp-server-abap`)
3. CI smoke test on all 3 platforms
4. Clear README instructions for ABAP developers

## Design

### 1. `--version` flag

Add to `main.go`: if `os.Args[1] == "--version"` or `-v`, print version and exit. This enables smoke testing and is useful for users.

```
$ mcp-server-abap --version
mcp-server-abap v1.2.3
```

### 2. Fix `.goreleaser.yaml`

- Change `owner: dachner` → `owner: Hochfrequenz`
- Keep existing build matrix (linux/darwin/windows, amd64/arm64)
- Archives: tar.gz for linux/darwin, zip for windows
- Checksums: `checksums.txt`

### 3. Fix `.github/workflows/release.yml`

- Update Go version: `1.22` → `1.26`
- Keep goreleaser action trigger on `v*` tags

### 4. Add `Dockerfile`

```dockerfile
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY mcp-server-abap /usr/local/bin/mcp-server-abap
ENTRYPOINT ["mcp-server-abap"]
```

Alpine-based (not scratch) for TLS root certs needed for HTTPS to SAP. Binary is copied from goreleaser build output. Users mount config:

```bash
docker run -v ./config.yaml:/config.yaml -e SAP_CONFIG_FILE=/config.yaml ghcr.io/hochfrequenz/mcp-server-abap
```

### 5. Add `.github/workflows/docker.yml`

Triggers on release tags (`v*`). Steps:
1. Checkout
2. Set up Go 1.26, build linux/amd64 binary
3. `docker/login-action` → GHCR
4. `docker/metadata-action` → tags (semver, latest)
5. `docker/build-push-action` → push to GHCR

### 6. Add smoke test to `.github/workflows/test.yml`

Add a job with matrix `[ubuntu-latest, macos-latest, windows-latest]`:
1. Checkout + setup Go
2. `go build -ldflags "-X main.version=test" -o mcp-server-abap .`
3. `./mcp-server-abap --version` → assert exit 0, output contains "test"

### 7. README updates

Add a clear "Getting Started" section:

```
## Getting started

### 1. Download
Download the binary for your OS from the [releases page](link).

### 2. Create config.yaml
<example with placeholders>

### 3. Add to Claude Desktop
<copy-paste JSON snippet>
```

## Files changed

| File | Action |
|------|--------|
| `main.go` | Add `--version` flag |
| `.goreleaser.yaml` | Fix org name |
| `.github/workflows/release.yml` | Fix Go version |
| `.github/workflows/docker.yml` | New — Docker build+push |
| `.github/workflows/test.yml` | Add smoke test matrix job |
| `Dockerfile` | New |
| `README.md` | Update install instructions |
