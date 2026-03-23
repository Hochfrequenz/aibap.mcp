# CI Pipeline & Branch Protection Design

## Goal

Introduce a comprehensive CI pipeline and branch protection for `mcp-server-abap`, modeled after [Hochfrequenz/go-template-repository](https://github.com/Hochfrequenz/go-template-repository). CodeQL is excluded by design.

## Files to Create

| File | Purpose |
|------|---------|
| `.github/workflows/test.yml` | Run unit tests on push + PR |
| `.github/workflows/coverage.yml` | Enforce minimum code coverage (70%) |
| `.github/workflows/golangci-lint.yml` | Lint + format check on PRs |
| `.github/workflows/no_byte_order_mark.yml` | BOM check on push to master + PRs |
| `.github/workflows/dependabot_automerge.yml` | Auto-approve + squash-merge Dependabot PRs |
| `.github/dependabot.yml` | Daily dependency update monitoring |
| `.github/CODEOWNERS` | Code ownership for review requirements |
| `.gitattributes` | Enforce LF line endings for Go files |

No existing files are modified. `release.yml` remains untouched.

## Workflow Details

### test.yml

- **Triggers:** push (all branches), pull_request
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Steps:**
  1. Checkout code
  2. Setup Go 1.26.x
  3. Run `go test ./...`

### coverage.yml

- **Triggers:** push (all branches), pull_request
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Steps:**
  1. Checkout code
  2. Setup Go 1.26.x
  3. Run `go test ./... -v -covermode=count -coverprofile=coverage.out`
  4. Convert to lcov format via `jandelgado/gcov2lcov-action`
  5. Enforce 70% minimum via `VeryGoodOpenSource/very_good_coverage`
- **Notes:** 70% is the initial threshold; increase over time as coverage improves.

### golangci-lint.yml

- **Triggers:** pull_request only
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Steps:**
  1. Checkout code
  2. Setup Go 1.26.x
  3. Run `golangci/golangci-lint-action` with enabled linters: `dupl`, `goconst`, `gocyclo`
  4. Run format check: `golangci-lint fmt --diff --enable gofmt`

### no_byte_order_mark.yml

- **Triggers:** push to master, pull_request
- **Runner:** ubuntu-latest
- **Steps:**
  1. Checkout code
  2. Run `arma-actions/bom-check@v1`

### dependabot_automerge.yml

- **Triggers:** pull_request from `dependabot[bot]`
- **Permissions:** contents write, pull-requests write
- **Steps:**
  1. Auto-approve PR via `gh pr review --approve`
  2. Auto-merge with squash via `gh pr merge --squash --auto`
- **Safety:** CI checks must pass before merge is executed (GitHub enforces this via branch protection required status checks).

## Dependency Management

### .github/dependabot.yml

- **Go modules:** daily update schedule, target branch `master`
- **GitHub Actions:** daily update schedule, target branch `master`

## Supporting Files

### .github/CODEOWNERS

```
* @hf-mrdachner
```

All files require review from `@hf-mrdachner`.

### .gitattributes

```
*.go text eol=lf
```

Ensures consistent line endings for Go source files across platforms.

## Branch Protection Rules (Manual Configuration)

Configure in GitHub repository settings for the `master` branch:

- **Require a pull request before merging**
- **Required status checks before merging:**
  - `test`
  - `coverage`
  - `golangci-lint`
  - `bom-check`
- **Require review from code owners**
- **Do not allow bypassing the above settings**

## Decisions

- **No CodeQL:** Excluded per user preference.
- **No Go proxy warmup:** Not needed at this stage.
- **Coverage at 70%:** Conservative starting point; raise once actual coverage is confirmed in CI.
- **Go 1.26.x everywhere:** Latest stable version used across all workflows.
- **Linters (dupl, goconst, gocyclo):** Matches reference repository; catches duplicated code, magic constants, and complex functions.
