package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

const abapGitCompanionURL = "https://github.com/Hochfrequenz/Z_ABAPGIT_PULL_MCP_SHORTCUT"

// Hints for the abapGit sync companion's error codes (#135). The companion's
// errors do not wrap *adt.ADTError, so none of the generic status hints apply.
const (
	abapGitCredentialsHint = "Do not retry. Tell the human to create or fix the SM59 HTTP destination `ZGIT_<SAP user>` " +
		"(connection type G, host github.com, port 443, SSL active, basic authentication with a fine-grained personal access token). " +
		"Setup: " + abapGitCompanionURL + "#readme"
	abapGitRemoteChangedHint = "The remote branch changed since the last pull. Run `abapgit_pull` first (with confirmation), " +
		"re-apply your local change, then push again. There is no merge."
	abapGitTransportRequiredHint = "Pass the `transport` parameter with a modifiable request."
	abapGitNoModifiableTaskHint  = "The calling user needs a modifiable task in that request. See `create_transport_task`."
	abapGitRepoLookupHint        = "Call `abapgit_list_repos` and pass the exact repository name or URL."
	abapGitObjectNotInRepoHint   = "The listed objects are not in the package of the repository."
	abapGitRequirementsHint      = "Resolve the missing requirements in the abapGit UI; this tool never decides them."
	abapGitGitErrorHint          = "The Git host refused the request. Check the branch, and retry only after `abapgit_list_repos` or an `abapgit_push` with `dry_run: true`."
	abapGitNotInstalledHint      = "Install the companion on this system: " + abapGitCompanionURL
)

// abapGitHintByCode maps companion error codes to recovery hints. BAD_REQUEST
// and INTERNAL have no special hint beyond the message.
var abapGitHintByCode = map[string]string{
	adt.AbapGitErrCredentialsMissing:  abapGitCredentialsHint,
	adt.AbapGitErrCredentialsRejected: abapGitCredentialsHint,
	adt.AbapGitErrRemoteChanged:       abapGitRemoteChangedHint,
	adt.AbapGitErrTransportRequired:   abapGitTransportRequiredHint,
	adt.AbapGitErrNoModifiableTask:    abapGitNoModifiableTaskHint,
	adt.AbapGitErrRepoNotFound:        abapGitRepoLookupHint,
	adt.AbapGitErrRepoAmbiguous:       abapGitRepoLookupHint,
	adt.AbapGitErrObjectNotInRepo:     abapGitObjectNotInRepoHint,
	adt.AbapGitErrRequirementsNotMet:  abapGitRequirementsHint,
	adt.AbapGitErrGitError:            abapGitGitErrorHint,
}

// abapGitSyncHint returns the hint for a companion error. ok is true for every
// companion error (even without a hint), so that no generic hint is added.
func abapGitSyncHint(err error) (hint string, ok bool) {
	var syncErr *adt.AbapGitSyncError
	if errors.As(err, &syncErr) {
		return abapGitHintByCode[syncErr.Code], true
	}
	if errors.Is(err, adt.ErrAbapGitSyncNotInstalled) {
		return abapGitNotInstalledHint, true
	}
	return "", false
}

// abapGitErrorResult is errorResult plus the companion's detail lines, which
// name the files or objects behind the error code.
func abapGitErrorResult(err error) *mcp.CallToolResult {
	var syncErr *adt.AbapGitSyncError
	if errors.As(err, &syncErr) && len(syncErr.Details) > 0 {
		err = fmt.Errorf("%w\n%s", err, strings.Join(syncErr.Details, "\n"))
	}
	return errorResult(err)
}

// decodeArgList converts a raw array argument (as mcp-go delivers it: []any of
// map[string]any) into a typed slice. Every entry must carry the required
// string fields. An absent argument yields an empty result without error.
func decodeArgList[T any](req mcp.CallToolRequest, name string, required ...string) ([]T, *mcp.CallToolResult) {
	raw, present := req.GetArguments()[name]
	if !present || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, errorResult(fmt.Errorf("parameter %q must be an array of objects", name))
	}
	out := make([]T, 0, len(list))
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, errorResult(fmt.Errorf("%s[%d] must be an object", name, i))
		}
		for _, field := range required {
			if v, _ := entry[field].(string); strings.TrimSpace(v) == "" {
				return nil, errorResult(fmt.Errorf("%s[%d].%s is required", name, i, field))
			}
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return nil, errorResult(fmt.Errorf("%s[%d]: %w", name, i, err))
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, errorResult(fmt.Errorf("%s[%d]: %w", name, i, err))
		}
		out = append(out, v)
	}
	return out, nil
}

var abapGitConfirmItemsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"obj_type": map[string]any{"type": "string", "description": "Object type, e.g. PROG, CLAS"},
		"obj_name": map[string]any{"type": "string", "description": "Object name"},
		"action":   map[string]any{"type": "string", "description": "The action to confirm, exactly as listed in confirmations_required"},
	},
	"required": []string{"obj_type", "obj_name", "action"},
}

var abapGitObjectItemsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"obj_type": map[string]any{"type": "string", "description": "Object type, e.g. PROG, CLAS"},
		"obj_name": map[string]any{"type": "string", "description": "Object name"},
	},
	"required": []string{"obj_type", "obj_name"},
}

const abapGitRepoParamDesc = "Repository name or URL, as listed by abapgit_list_repos"

func registerAbapGitSyncTools(s toolAdder, client adt.AbapGitSyncClient) {
	s.AddTool(mcp.NewTool("abapgit_list_repos",
		mcp.WithTitleAnnotation("List abapGit Repositories"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(
			"List the abapGit repositories known on the SAP system (key, name, URL, package, branch, last deserialization time). "+
				"Requires the companion ABAP package: "+abapGitCompanionURL+". "+
				"Use `deserialized_at` to check whether a pull or push that timed out went through."),
		mcp.WithOutputSchema[adt.AbapGitRepoList](),
	), func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := client.ListAbapGitRepos(ctx)
		if err != nil {
			return abapGitErrorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	})

	s.AddTool(mcp.NewTool("abapgit_pull",
		mcp.WithTitleAnnotation("abapGit Pull"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Pull an abapGit repository from its Git remote into the SAP system. This overwrites the current state of the repository objects on SAP "+
				"and deletes objects that were deleted in Git (both only after confirmation). "+
				"Requires the companion ABAP package: "+abapGitCompanionURL+". "+
				"If the result has status `needs_confirmation`, nothing was changed yet: show the human the listed objects and the per-file states, "+
				"and only after the human agrees call again with the same `repo` and the entries to accept in `confirm`. "+
				"On CREDENTIALS_MISSING or CREDENTIALS_REJECTED do not retry: tell the human to create or fix the SM59 destination `ZGIT_<SAP user>` "+
				"(setup: "+abapGitCompanionURL+"#readme). "+
				"Never retry a pull automatically. After a timeout the outcome is unknown: check `abapgit_list_repos` (`deserialized_at`) first."),
		mcp.WithString("repo", mcp.Required(), mcp.Description(abapGitRepoParamDesc)),
		mcp.WithString("transport",
			mcp.Description("Transport request to record the changes in. Required when the package of the repository is not local.")),
		mcp.WithArray("confirm",
			mcp.Description("Confirmations for objects reported in confirmations_required. Each entry: {obj_type, obj_name, action}."),
			mcp.Items(abapGitConfirmItemsSchema)),
		mcp.WithOutputSchema[adt.AbapGitPullResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, errRes := requireString(req, "repo")
		if errRes != nil {
			return errRes, nil
		}
		confirm, errRes := decodeArgList[adt.AbapGitConfirmation](req, "confirm", "obj_type", "obj_name", "action")
		if errRes != nil {
			return errRes, nil
		}
		res, err := client.PullAbapGitRepo(ctx, adt.AbapGitPullRequest{
			Repo:      repo,
			Transport: strings.TrimSpace(req.GetString("transport", "")),
			Confirm:   confirm,
		})
		if err != nil {
			return abapGitErrorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	})

	s.AddTool(mcp.NewTool("abapgit_push",
		mcp.WithTitleAnnotation("abapGit Push"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Commit the given objects from the SAP system to the Git remote of the repository and push. This publishes to an external Git host. "+
				"Requires the companion ABAP package: "+abapGitCompanionURL+". "+
				"Run with `dry_run: true` first: it lists what would be committed and also performs the credential check. "+
				"If the remote branch changed since the last pull, the push fails with REMOTE_CHANGED: pull first, re-apply the change, push again (there is no merge). "+
				"On CREDENTIALS_MISSING or CREDENTIALS_REJECTED do not retry: tell the human to create or fix the SM59 destination `ZGIT_<SAP user>` "+
				"(setup: "+abapGitCompanionURL+"#readme). "+
				"Never retry a push automatically. After a timeout the outcome is unknown: check `abapgit_list_repos` (`deserialized_at`) or run a dry run."),
		mcp.WithString("repo", mcp.Required(), mcp.Description(abapGitRepoParamDesc)),
		mcp.WithArray("objects", mcp.Required(),
			mcp.Description("Objects to commit. Each entry: {obj_type, obj_name}. Must not be empty."),
			mcp.Items(abapGitObjectItemsSchema)),
		mcp.WithString("message", mcp.Required(), mcp.Description("Commit message")),
		mcp.WithBoolean("dry_run",
			mcp.Description("true = report what would be committed and check credentials, change nothing. Default false.")),
		mcp.WithOutputSchema[adt.AbapGitPushResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, errRes := requireString(req, "repo")
		if errRes != nil {
			return errRes, nil
		}
		message, errRes := requireString(req, "message")
		if errRes != nil {
			return errRes, nil
		}
		objects, errRes := decodeArgList[adt.AbapGitObjectRef](req, "objects", "obj_type", "obj_name")
		if errRes != nil {
			return errRes, nil
		}
		if len(objects) == 0 {
			return errorResult(errors.New("parameter \"objects\" must contain at least one {obj_type, obj_name} entry")), nil
		}
		res, err := client.PushAbapGitRepo(ctx, adt.AbapGitPushRequest{
			Repo:    repo,
			Objects: objects,
			Message: message,
			DryRun:  req.GetBool("dry_run", false),
		})
		if err != nil {
			return abapGitErrorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	})
}
