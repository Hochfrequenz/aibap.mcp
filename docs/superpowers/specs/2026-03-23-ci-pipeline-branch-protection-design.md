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
| `.gitattributes` | Enforce LF line endings for all text files |

## Files to Modify

| File | Change |
|------|--------|
| `README.md` | Add status badges at the top |
| `go.mod` | Update Go version directive to 1.26 |

`release.yml` remains untouched.

## Workflow Details

All workflows use `actions/checkout@v6` and `actions/setup-go@v6` (latest versions, matching go-bo4e).

### test.yml

- **Workflow name:** `Unittests` (used for badge URL)
- **Triggers:** push (all branches), pull_request
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Job ID:** `test`
- **Steps:**
  1. Checkout code (`actions/checkout@v6`)
  2. Setup Go (`actions/setup-go@v6`, go-version `1.26.x`)
  3. Run `go test ./...`

### coverage.yml

- **Workflow name:** `coverage`
- **Triggers:** push (all branches), pull_request
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Job ID:** `coverage`
- **Steps:**
  1. Checkout code (`actions/checkout@v6`)
  2. Setup Go (`actions/setup-go@v6`, go-version `1.26.x`)
  3. Run `go test ./... -v -covermode=count -coverprofile=coverage.out`
  4. Convert to lcov format via `jandelgado/gcov2lcov-action@v1.2.0` (outputs `coverage.lcov`)
  5. Enforce 70% minimum via `VeryGoodOpenSource/very_good_coverage@v3.0.0` with `min_coverage: 70`, `path: coverage.lcov`
- **Notes:** 70% is the initial threshold; increase over time as coverage improves.

### golangci-lint.yml

- **Workflow name:** `golangci-lint`
- **Triggers:** pull_request only
- **Runner:** ubuntu-latest
- **Go version:** 1.26.x
- **Job ID:** `golangci-lint`
- **Steps:**
  1. Checkout code (`actions/checkout@v6`)
  2. Setup Go (`actions/setup-go@v6`, go-version `1.26.x`)
  3. Run `golangci/golangci-lint-action@v9` with additional linters enabled: `dupl`, `goconst`, `gocyclo` (in addition to defaults)
  4. Run format check: `golangci-lint fmt --diff --enable gofmt` (requires golangci-lint v2+; the action `@v9` uses v2)
- **Note:** PR-only trigger is intentional — linting on every push to feature branches adds noise; the PR gate is sufficient.

### no_byte_order_mark.yml

- **Workflow name:** `Prevent ByteOrderMarks`
- **Triggers:** push to master (`branches: [master]`), pull_request
- **Runner:** ubuntu-latest
- **Job ID:** `bom-check`
- **Steps:**
  1. Checkout code (`actions/checkout@v6`)
  2. Run `arma-actions/bom-check@v1`

### dependabot_automerge.yml

- **Triggers:** `pull_request_target` (not `pull_request`, for security with write permissions)
- **Condition:** `if: github.actor == 'dependabot[bot]'`
- **Permissions:** contents write, pull-requests write
- **Job ID:** `automerge`
- **Environment variables:**
  - `PR_URL: ${{github.event.pull_request.html_url}}`
  - `GITHUB_TOKEN: ${{secrets.GITHUB_TOKEN}}`
- **Steps:**
  1. Auto-approve PR via `gh pr review --approve "$PR_URL"`
  2. Auto-merge with squash via `gh pr merge --squash --auto "$PR_URL"`
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
* text=auto
*.go text eol=lf
```

`text=auto` normalizes line endings for all text files. `*.go` explicitly forces LF (relevant for Windows development).

## README Badges

Add status badges at the top of `README.md`, before the title, following the go-bo4e pattern:

```markdown
![Unittest status badge](https://github.com/dachner/mcp-server-abap/workflows/Unittests/badge.svg)
![Coverage status badge](https://github.com/dachner/mcp-server-abap/workflows/coverage/badge.svg)
![Linter status badge](https://github.com/dachner/mcp-server-abap/workflows/golangci-lint/badge.svg)
```

Badge URLs use the workflow `name` field, not the filename. This is why workflow names are specified explicitly above.

## Branch Protection Rules (Manual Configuration)

Configure in GitHub repository settings for the `master` branch:

- **Require a pull request before merging**
- **Required status checks before merging** (these match the job IDs above):
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
- **Go 1.26.x:** Latest stable version. `go.mod` will be updated to match. `release.yml` still uses 1.22 but is not modified per user request.
- **Linters (dupl, goconst, gocyclo):** Added on top of defaults; matches reference repository. Catches duplicated code, magic constants, and complex functions.
- **golangci-lint PR-only:** Intentional to reduce noise on feature branch pushes; PR gate is the quality checkpoint.
- **`pull_request_target` for Dependabot:** Deliberate departure from reference repo (which uses `pull_request`). Required for security when running with write permissions on bot-created PRs.
- **Daily Dependabot schedule:** Matches reference repository. Combined with auto-merge, keeps dependencies current with minimal manual effort.
- **No `.golangci.yml`:** Linter config is passed via CLI flags in the workflow, matching the reference repo's approach. Can be extracted to a config file later if needed.
- **Action versions:** Uses `actions/checkout@v6`, `actions/setup-go@v6`, `golangci/golangci-lint-action@v9` with golangci-lint `v2.9.0` (all latest, matching go-bo4e). Dependabot will propose updates if newer versions become available.
