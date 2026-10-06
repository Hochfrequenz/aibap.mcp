package tools_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func abapGitToolError(t *testing.T, tool string, args map[string]interface{}, mock *mockClient) string {
	t.Helper()
	res := callTool(t, newTestServer(mock), tool, args)
	if !res.IsError {
		t.Fatalf("%s: expected an error result, got: %s", tool, abapGitText(res))
	}
	return abapGitText(res)
}

func TestAbapGitListRepos_ReturnsCountAndRepos(t *testing.T) {
	mock := &mockClient{listAbapGitReposFn: func(context.Context) (*adt.AbapGitRepoList, error) {
		return &adt.AbapGitRepoList{Count: 1, Repos: []adt.AbapGitRepo{{Key: "K1", Name: "example", URL: "https://github.com/example/repo"}}}, nil
	}}
	res := callTool(t, newTestServer(mock), "abapgit_list_repos", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", abapGitText(res))
	}
	sc, ok := res.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatalf("structuredContent is %T, want object", res.StructuredContent)
	}
	if fmt.Sprint(sc["count"]) != "1" {
		t.Errorf("count = %v, want 1", sc["count"])
	}
	if _, ok := sc["repos"]; !ok {
		t.Errorf("repos missing in %v", sc)
	}
}

func TestAbapGitListRepos_AdtlerErrorBecomesToolError(t *testing.T) {
	mock := &mockClient{listAbapGitReposFn: func(context.Context) (*adt.AbapGitRepoList, error) {
		return nil, errors.New("boom")
	}}
	if text := abapGitToolError(t, "abapgit_list_repos", map[string]interface{}{}, mock); !strings.Contains(text, "boom") {
		t.Errorf("error text %q lacks the adtler message", text)
	}
}

func TestAbapGitPull_MapsArgumentsAndConfirm(t *testing.T) {
	var got adt.AbapGitPullRequest
	mock := &mockClient{pullAbapGitRepoFn: func(_ context.Context, r adt.AbapGitPullRequest) (*adt.AbapGitPullResult, error) {
		got = r
		return &adt.AbapGitPullResult{Status: adt.AbapGitStatusNeedsConfirmation,
			ConfirmationsRequired: []adt.AbapGitConfirmationRequired{{ObjType: "PROG", ObjName: "ZEXAMPLE", Action: "overwrite", Files: []adt.AbapGitFileState{}}},
			Log:                   []adt.AbapGitLogEntry{}}, nil
	}}
	res := callTool(t, newTestServer(mock), "abapgit_pull", map[string]interface{}{
		"repo":      "https://github.com/example/repo",
		"transport": "<request>",
		"confirm": []interface{}{
			map[string]interface{}{"obj_type": "PROG", "obj_name": "ZEXAMPLE", "action": "overwrite"},
		},
	})
	if res.IsError {
		t.Fatalf("needs_confirmation must be a success result, got: %s", abapGitText(res))
	}
	want := adt.AbapGitPullRequest{Repo: "https://github.com/example/repo", Transport: "<request>",
		Confirm: []adt.AbapGitConfirmation{{ObjType: "PROG", ObjName: "ZEXAMPLE", Action: "overwrite"}}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("request = %+v, want %+v", got, want)
	}
	if !strings.Contains(abapGitText(res), "confirmations_required") {
		t.Errorf("result text lacks confirmations_required: %s", abapGitText(res))
	}
}

func TestAbapGitPush_MapsArgumentsAndDryRunDefault(t *testing.T) {
	var got adt.AbapGitPushRequest
	mock := &mockClient{pushAbapGitRepoFn: func(_ context.Context, r adt.AbapGitPushRequest) (*adt.AbapGitPushResult, error) {
		got = r
		return &adt.AbapGitPushResult{Status: adt.AbapGitStatusPushed, Files: []adt.AbapGitPushFile{}}, nil
	}}
	args := map[string]interface{}{
		"repo":    "example",
		"message": "msg",
		"objects": []interface{}{map[string]interface{}{"obj_type": "CLAS", "obj_name": "ZCL_EXAMPLE"}},
	}
	if res := callTool(t, newTestServer(mock), "abapgit_push", args); res.IsError {
		t.Fatalf("unexpected error: %s", abapGitText(res))
	}
	if got.DryRun {
		t.Error("dry_run must default to false")
	}
	if got.Repo != "example" || got.Message != "msg" || len(got.Objects) != 1 || got.Objects[0] != (adt.AbapGitObjectRef{ObjType: "CLAS", ObjName: "ZCL_EXAMPLE"}) {
		t.Errorf("request = %+v", got)
	}
	args["dry_run"] = true
	if res := callTool(t, newTestServer(mock), "abapgit_push", args); res.IsError {
		t.Fatalf("unexpected error: %s", abapGitText(res))
	}
	if !got.DryRun {
		t.Error("dry_run: true was not passed through")
	}
}

func TestAbapGitValidation_RejectsBeforeCallingAdtler(t *testing.T) {
	obj := []interface{}{map[string]interface{}{"obj_type": "CLAS", "obj_name": "ZCL_EXAMPLE"}}
	cases := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{"pull without repo", "abapgit_pull", map[string]interface{}{}},
		{"push without repo", "abapgit_push", map[string]interface{}{"message": "m", "objects": obj}},
		{"push empty objects", "abapgit_push", map[string]interface{}{"repo": "r", "message": "m", "objects": []interface{}{}}},
		{"push without objects", "abapgit_push", map[string]interface{}{"repo": "r", "message": "m"}},
		{"push empty message", "abapgit_push", map[string]interface{}{"repo": "r", "message": " ", "objects": obj}},
		{"push object without name", "abapgit_push", map[string]interface{}{"repo": "r", "message": "m", "objects": []interface{}{map[string]interface{}{"obj_type": "CLAS"}}}},
		{"pull confirm without action", "abapgit_pull", map[string]interface{}{"repo": "r", "confirm": []interface{}{map[string]interface{}{"obj_type": "PROG", "obj_name": "ZEXAMPLE"}}}},
		{"pull confirm not an object", "abapgit_pull", map[string]interface{}{"repo": "r", "confirm": []interface{}{"PROG"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			mock := &mockClient{
				pullAbapGitRepoFn: func(context.Context, adt.AbapGitPullRequest) (*adt.AbapGitPullResult, error) {
					called = true
					return nil, nil
				},
				pushAbapGitRepoFn: func(context.Context, adt.AbapGitPushRequest) (*adt.AbapGitPushResult, error) {
					called = true
					return nil, nil
				},
			}
			abapGitToolError(t, tc.tool, tc.args, mock)
			if called {
				t.Error("adtler was called despite invalid input")
			}
		})
	}
}

func TestAbapGitErrors_TextDetailsAndHints(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		contains []string
	}{
		{"remote changed with details", &adt.AbapGitSyncError{HTTPStatus: 409, Code: adt.AbapGitErrRemoteChanged, Message: "m", Details: []string{"/src/a.prog.abap _M", "/src/b.prog.abap M_"}},
			[]string{"REMOTE_CHANGED: m", "/src/a.prog.abap _M", "/src/b.prog.abap M_", "Hint:", "no merge"}},
		{"credentials missing", &adt.AbapGitSyncError{HTTPStatus: 403, Code: adt.AbapGitErrCredentialsMissing, Message: "no destination"},
			[]string{"CREDENTIALS_MISSING: no destination", "ZGIT_", "Do not retry", "https://github.com/Hochfrequenz/Z_ABAPGIT_PULL_MCP_SHORTCUT#readme"}},
		{"credentials rejected", &adt.AbapGitSyncError{HTTPStatus: 403, Code: adt.AbapGitErrCredentialsRejected, Message: "rejected"},
			[]string{"ZGIT_", "Do not retry"}},
		{"repo not found", &adt.AbapGitSyncError{HTTPStatus: 404, Code: adt.AbapGitErrRepoNotFound, Message: "x"},
			[]string{"abapgit_list_repos"}},
		{"transport required", &adt.AbapGitSyncError{HTTPStatus: 400, Code: adt.AbapGitErrTransportRequired, Message: "x"},
			[]string{"`transport`"}},
		{"no modifiable task", &adt.AbapGitSyncError{HTTPStatus: 409, Code: adt.AbapGitErrNoModifiableTask, Message: "x"},
			[]string{"create_transport_task"}},
		{"not installed, wrapped", fmt.Errorf("ListAbapGitRepos: %w", adt.ErrAbapGitSyncNotInstalled),
			[]string{"Hint:", "https://github.com/Hochfrequenz/Z_ABAPGIT_PULL_MCP_SHORTCUT"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockClient{pullAbapGitRepoFn: func(context.Context, adt.AbapGitPullRequest) (*adt.AbapGitPullResult, error) {
				return nil, tc.err
			}}
			text := abapGitToolError(t, "abapgit_pull", map[string]interface{}{"repo": "r"}, mock)
			for _, want := range tc.contains {
				if !strings.Contains(text, want) {
					t.Errorf("error text lacks %q:\n%s", want, text)
				}
			}
			// Companion errors do not wrap *adt.ADTError: no generic status hint.
			for _, generic := range []string{"search_objects", "SM21", "S_DEVELOP"} {
				if strings.Contains(text, generic) {
					t.Errorf("error text carries generic hint %q:\n%s", generic, text)
				}
			}
		})
	}
}

func abapGitText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
