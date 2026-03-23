# CI Pipeline & Branch Protection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CI workflows (tests, coverage, linting, BOM check, Dependabot), supporting files (CODEOWNERS, .gitattributes), and README badges to mcp-server-abap.

**Architecture:** All quality gates are GitHub Actions workflows triggered on push/PR. No local hooks. Branch protection is configured manually in GitHub settings after workflows exist.

**Tech Stack:** GitHub Actions, Go 1.26.x, golangci-lint v2.9.0, gcov2lcov, VeryGoodOpenSource/very_good_coverage

**Spec:** `docs/superpowers/specs/2026-03-23-ci-pipeline-branch-protection-design.md`

---

### Task 1: Create feature branch and supporting files

**Files:**
- Create: `.gitattributes`
- Create: `.github/CODEOWNERS`

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feat/ci-pipeline
```

- [ ] **Step 2: Create `.gitattributes`**

```
* text=auto
*.go text eol=lf
```

- [ ] **Step 3: Create `.github/CODEOWNERS`**

```
* @hf-mrdachner
```

- [ ] **Step 4: Commit**

```bash
git add .gitattributes .github/CODEOWNERS
git commit -m "chore: add .gitattributes and CODEOWNERS"
```

---

### Task 2: Create test workflow

**Files:**
- Create: `.github/workflows/test.yml`

- [ ] **Step 1: Create `.github/workflows/test.yml`**

```yaml
name: Unittests

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version: '1.26.x'

      - name: Run tests
        run: go test ./...
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: add unit test workflow"
```

---

### Task 3: Create coverage workflow

**Files:**
- Create: `.github/workflows/coverage.yml`

- [ ] **Step 1: Create `.github/workflows/coverage.yml`**

```yaml
name: coverage

on:
  push:
  pull_request:

jobs:
  coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version: '1.26.x'

      - name: Run tests with coverage
        run: go test ./... -v -covermode=count -coverprofile=coverage.out

      - name: Convert coverage to lcov
        uses: jandelgado/gcov2lcov-action@v1.2.0

      - name: Check coverage threshold
        uses: VeryGoodOpenSource/very_good_coverage@v3.0.0
        with:
          path: coverage.lcov
          min_coverage: 70
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/coverage.yml
git commit -m "ci: add coverage workflow with 70% minimum threshold"
```

---

### Task 4: Create golangci-lint workflow

**Files:**
- Create: `.github/workflows/golangci-lint.yml`

- [ ] **Step 1: Create `.github/workflows/golangci-lint.yml`**

```yaml
name: golangci-lint

on:
  pull_request:

jobs:
  golangci-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version: '1.26.x'

      - name: Lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.9.0
          args: --enable dupl,goconst,gocyclo

      - name: Check formatting
        run: golangci-lint fmt --diff --enable gofmt
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/golangci-lint.yml
git commit -m "ci: add golangci-lint workflow"
```

---

### Task 5: Create BOM check workflow

**Files:**
- Create: `.github/workflows/no_byte_order_mark.yml`

- [ ] **Step 1: Create `.github/workflows/no_byte_order_mark.yml`**

```yaml
name: Prevent ByteOrderMarks

on:
  push:
    branches: [master]
  pull_request:

jobs:
  bom-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - name: Check for BOM
        uses: arma-actions/bom-check@v1
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/no_byte_order_mark.yml
git commit -m "ci: add byte order mark check workflow"
```

---

### Task 6: Create Dependabot config and automerge workflow

**Files:**
- Create: `.github/dependabot.yml`
- Create: `.github/workflows/dependabot_automerge.yml`

- [ ] **Step 1: Create `.github/dependabot.yml`**

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "daily"
    target-branch: "master"

  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "daily"
    target-branch: "master"
```

- [ ] **Step 2: Create `.github/workflows/dependabot_automerge.yml`**

```yaml
name: Dependabot auto-merge

on:
  pull_request_target:

permissions:
  contents: write
  pull-requests: write

jobs:
  automerge:
    runs-on: ubuntu-latest
    if: github.actor == 'dependabot[bot]'
    env:
      PR_URL: ${{github.event.pull_request.html_url}}
      GITHUB_TOKEN: ${{secrets.GITHUB_TOKEN}}
    steps:
      - name: Approve PR
        run: gh pr review --approve "$PR_URL"

      - name: Auto-merge
        run: gh pr merge --squash --auto "$PR_URL"
```

- [ ] **Step 3: Commit**

```bash
git add .github/dependabot.yml .github/workflows/dependabot_automerge.yml
git commit -m "ci: add Dependabot config with auto-merge"
```

---

### Task 7: Update go.mod and add README badges

**Files:**
- Modify: `go.mod` (line 3: change `go 1.23.0` to `go 1.26`)
- Modify: `README.md` (insert badges before line 1)

- [ ] **Step 1: Update `go.mod` Go version directive**

Change line 3 from:
```
go 1.23.0
```
to:
```
go 1.26
```

- [ ] **Step 2: Add status badges to top of `README.md`**

Insert before the `# mcp-server-abap` heading:

```markdown
![Unittest status badge](https://github.com/dachner/mcp-server-abap/workflows/Unittests/badge.svg)
![Coverage status badge](https://github.com/dachner/mcp-server-abap/workflows/coverage/badge.svg)
![Linter status badge](https://github.com/dachner/mcp-server-abap/workflows/golangci-lint/badge.svg)

```

- [ ] **Step 3: Commit**

```bash
git add go.mod README.md
git commit -m "chore: update Go to 1.26, add CI status badges to README"
```

---

### Task 8: Push branch and create PR

- [ ] **Step 1: Push feature branch**

```bash
git push -u origin feat/ci-pipeline
```

- [ ] **Step 2: Create PR**

```bash
gh pr create --title "Add CI pipeline and branch protection" --body "$(cat <<'EOF'
## Summary
- Add GitHub Actions workflows: unit tests, coverage (70% min), golangci-lint, BOM check
- Add Dependabot with auto-approve + squash-merge
- Add CODEOWNERS (@hf-mrdachner) and .gitattributes
- Add CI status badges to README
- Update Go version to 1.26

## Branch protection (manual)
After merging, configure branch protection on `master`:
- Require PR before merging
- Required status checks: `test`, `coverage`, `golangci-lint`, `bom-check`
- Require review from code owners

## Reference
Modeled after [Hochfrequenz/go-template-repository](https://github.com/Hochfrequenz/go-template-repository) (minus CodeQL).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Verify CI runs on the PR**

Check that workflows trigger and pass. If any fail, fix and push.
