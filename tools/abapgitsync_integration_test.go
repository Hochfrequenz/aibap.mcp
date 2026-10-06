//go:build integration

package tools_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

// abapgitsync_integration_test.go exercises the abapgit_* tools against the
// live systems. They need the Z_ABAPGIT_PULL_MCP_SHORTCUT companion on the
// target system and skip when it is missing. The pull and push tests also
// need the AIBAP_ABAPGIT_* environment variables documented in README.md and
// skip when they are unset. Only counts and status values are logged.

// abapGitNotInstalledText is the message of adt.ErrAbapGitSyncNotInstalled.
const abapGitNotInstalledText = "companion is not installed"

// skipIfAbapGitNotInstalled skips the test when the tool error says the
// companion is not installed on the selected system.
func skipIfAbapGitNotInstalled(t *testing.T, sys string, res *mcp.CallToolResult) {
	t.Helper()
	if res.IsError && strings.Contains(textOf(res), abapGitNotInstalledText) {
		t.Skipf("abapGit sync companion not installed on %s", sys)
	}
}

// structuredInto decodes the structuredContent of a successful result into out.
func structuredInto(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	if res.StructuredContent == nil {
		t.Fatalf("result has no structuredContent: %s", textOf(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structuredContent: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode structuredContent %s: %v", raw, err)
	}
}

func TestAbapGitListRepos_Integration(t *testing.T) {
	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "abapgit_list_repos", map[string]interface{}{})
			skipIfAbapGitNotInstalled(t, sys, res)
			if res.IsError {
				t.Fatalf("abapgit_list_repos returned IsError=true: %s", textOf(res))
			}
			var list adt.AbapGitRepoList
			structuredInto(t, res, &list)
			if list.Count != len(list.Repos) {
				t.Errorf("count = %d but repos has %d entries", list.Count, len(list.Repos))
			}
			t.Logf("system=%s repos=%d", sys, list.Count)
		})
	}
}

func TestAbapGitPull_Integration(t *testing.T) {
	repo := os.Getenv("AIBAP_ABAPGIT_PULL_TEST_REPO")
	if repo == "" {
		t.Skip("AIBAP_ABAPGIT_PULL_TEST_REPO not set")
	}
	transport := os.Getenv("AIBAP_ABAPGIT_PULL_TEST_TRANSPORT")

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			args := map[string]interface{}{"repo": repo}
			if transport != "" {
				args["transport"] = transport
			}
			res := callTool(t, sharedServer, "abapgit_pull", args)
			skipIfAbapGitNotInstalled(t, sys, res)
			if res.IsError {
				t.Fatalf("abapgit_pull returned IsError=true: %s", textOf(res))
			}
			var pull adt.AbapGitPullResult
			structuredInto(t, res, &pull)
			switch pull.Status {
			case "pulled", "needs_confirmation":
			default:
				t.Fatalf("unexpected status %q; want pulled or needs_confirmation", pull.Status)
			}
			t.Logf("system=%s status=%s confirmations_required=%d log=%d",
				sys, pull.Status, len(pull.ConfirmationsRequired), len(pull.Log))
		})
	}
}

func TestAbapGitPushDryRun_Integration(t *testing.T) {
	repo := os.Getenv("AIBAP_ABAPGIT_PUSH_TEST_REPO")
	object := os.Getenv("AIBAP_ABAPGIT_PUSH_TEST_OBJECT")
	if repo == "" || object == "" {
		t.Skip("AIBAP_ABAPGIT_PUSH_TEST_REPO and AIBAP_ABAPGIT_PUSH_TEST_OBJECT must both be set")
	}
	parts := strings.Fields(object)
	if len(parts) != 2 {
		t.Fatalf("AIBAP_ABAPGIT_PUSH_TEST_OBJECT must be %q, got %d fields", "TYPE NAME", len(parts))
	}

	for _, sys := range integrationSystems {
		t.Run(sys, func(t *testing.T) {
			requireReachable(t, sys)
			mustSelectSystem(t, sharedServer, sys)

			res := callTool(t, sharedServer, "abapgit_push", map[string]interface{}{
				"repo": repo,
				"objects": []interface{}{
					map[string]interface{}{"obj_type": parts[0], "obj_name": parts[1]},
				},
				"message": "integration test dry run",
				"dry_run": true,
			})
			skipIfAbapGitNotInstalled(t, sys, res)
			if res.IsError {
				if strings.Contains(textOf(res), "CREDENTIALS_MISSING") {
					t.Logf("system=%s outcome=CREDENTIALS_MISSING (accepted)", sys)
					return
				}
				t.Fatalf("abapgit_push dry run returned IsError=true: %s", textOf(res))
			}
			var push adt.AbapGitPushResult
			structuredInto(t, res, &push)
			switch push.Status {
			case "dry_run", "nothing_to_push":
			default:
				t.Fatalf("unexpected status %q; want dry_run or nothing_to_push", push.Status)
			}
			t.Logf("system=%s status=%s files=%d", sys, push.Status, len(push.Files))
		})
	}
}
