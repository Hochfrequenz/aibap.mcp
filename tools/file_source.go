package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

func registerFileSourceTools(s toolAdder, client interface {
	adt.SourceClient
	adt.LockClient
	adt.SystemClient
}, lockMap *adt.LockMap, tracker *sessionLockTracker, selector SystemSelector) {
	s.AddTool(mcp.NewTool("set_source_from_file",
		mcp.WithTitleAnnotation("Set Source from File"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Upload ABAP source code from a local file to SAP. Auto-locks if needed (lock is retained for subsequent operations). A class include URI (/sap/bc/adt/oo/classes/<class>/includes/<testclasses|definitions|implementations|macros>) writes that include under a lock on the class; the file is sent byte for byte, unlike set_include_source's inline argument."),
		mcp.WithString(paramObjectURI,
			mcp.Required(),
			mcp.Description(descADTObjectURI),
		),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to source file (absolute or relative to working directory)"),
		),
		mcp.WithString("transport",
			mcp.Description("Transport request number (required for non-local packages)"),
		),
		mcp.WithString("lock_handle",
			mcp.Description("Explicit lock handle (optional, looked up from lock map)"),
		),
		mcp.WithOutputSchema[SetSourceFromFileResult](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uri := req.GetString(paramObjectURI, "")
		filePath := req.GetString("file_path", "")
		transport := req.GetString("transport", "")
		explicitHandle := req.GetString("lock_handle", "")
		// A class include has no lock of its own: the class is locked (#621).
		target := lockTargetURI(uri)
		classURI, include, isInclude := classIncludeOf(target)
		if isInclude {
			target = classURI
		}
		key, lockURI := lockKeyFor(selector.ActiveName(), target)

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			return errorResult(fmt.Errorf("reading file: %w", err)), nil
		}
		source := string(data)

		// Resolve lock handle: explicit param > lock map > auto-lock. On ECC,
		// the cached handle is never trusted (#377) — see resolveWriteLockHandle.
		// Track whether this call acquired the concrete SAP lock handle it is
		// about to use, so a later failure can roll back only that handle (#383)
		// — never release a caller-owned or merely reused cached lock.
		lockHandle, _, releaseOnFailure, err := resolveWriteLockHandle(ctx, client, lockMap, tracker, key, lockURI, explicitHandle, true)
		if err != nil {
			return errorResult(fmt.Errorf("auto-lock failed: %w", err)), nil
		}

		if isInclude {
			// No ETag: under a lock adtler's SetIncludeSource sends no
			// If-Match, because the include's own ETag is not the one SAP
			// checks on a write (#436). The generic path below fetched an ETag
			// from "<include URI>/source/main", which does not exist (404, #621).
			newETag, err := client.SetIncludeSource(ctx, classURI, include, source, lockHandle, transport, "")
			if err != nil {
				if releaseOnFailure {
					releaseAutoLock(ctx, client, lockMap, tracker, key, lockURI, lockHandle)
				}
				return errorResult(err), nil
			}
			return mcp.NewToolResultJSON(SetSourceFromFileResult{
				Success:    true,
				Lines:      len(strings.Split(source, "\n")),
				Locked:     true,
				LockHandle: lockHandle,
				ETag:       newETag,
			})
		}

		// Get ETag if not in lock map.
		etag, err := lockMap.ResolveETag(ctx, client, key, uri)
		if err != nil {
			if releaseOnFailure {
				releaseAutoLock(ctx, client, lockMap, tracker, key, lockURI, lockHandle)
			}
			return errorResult(err), nil
		}

		// Write source
		newETag, err := client.SetSource(ctx, uri, source, lockHandle, transport, etag)
		if err != nil {
			if releaseOnFailure {
				releaseAutoLock(ctx, client, lockMap, tracker, key, lockURI, lockHandle)
			}
			return errorResult(err), nil
		}
		lockMap.UpdateETag(key, newETag)

		lineCount := len(strings.Split(source, "\n"))
		return mcp.NewToolResultJSON(SetSourceFromFileResult{
			Success:    true,
			Lines:      lineCount,
			Locked:     true,
			LockHandle: lockHandle,
			ETag:       newETag,
		})
	})
}

// classIncludeOf splits a class include URI
// (/sap/bc/adt/oo/classes/<class>/includes/<include>) into the class URI and
// the include name. adtler validates the name (testclasses, definitions,
// implementations, macros).
func classIncludeOf(uri string) (classURI, include string, ok bool) {
	const classes, marker = "/sap/bc/adt/oo/classes/", "/includes/"
	if !strings.HasPrefix(strings.ToLower(uri), classes) {
		return "", "", false
	}
	rest := uri[len(classes):]
	i := strings.Index(strings.ToLower(rest), marker)
	if i < 0 {
		return "", "", false
	}
	class, include := rest[:i], rest[i+len(marker):]
	if class == "" || include == "" || strings.ContainsAny(class+include, "/?#") {
		return "", "", false
	}
	return uri[:len(classes)] + class, strings.ToLower(include), true
}
