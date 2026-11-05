package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

const verifyCopiesHint = "SAP reports success even when it could not lock the target, so read the target " +
	"back with get_transport_objects to confirm. "

const copiesFallbackHint = "Requires ABAP Platform 2105 or later and a modifiable target; on older systems " +
	"(including ECC on SAP_BASIS 7.50) the call goes through the configured fallback if it supports this " +
	"(via_fallback=true), otherwise include the objects in SE09 via the sapgui.mcp server."

// registerTransportCopiesTools adds the tools that fill a request, usually a
// transport of copies, from another request or from a package (#602).
func registerTransportCopiesTools(s toolAdder, client adt.TransportClient, fallback BlackMagicClient) {
	s.AddTool(mcp.NewTool("copy_transport_objects",
		mcp.WithTitleAnnotation("Copy Transport Objects"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Copy the object entries of one transport request into another, as SE09's \"include objects\" does. "+
				"The usual target is a transport of copies (create_transport with category T). For a modifiable "+
				"source the request and each of its tasks are copied; a released source is copied as one. "+
				verifyCopiesHint+copiesFallbackHint,
		),
		mcp.WithString("source_request", mcp.Required(), mcp.Description("Request whose objects are copied, e.g. DEVK900123")),
		mcp.WithString("target_request", mcp.Required(), mcp.Description("Modifiable request that receives the entries, e.g. DEVK900124")),
		mcp.WithBoolean("with_documentation", mcp.Description("Also copy the source's documentation (default: true, as in SE09)")),
		mcp.WithOutputSchema[CopyTransportObjectsResult](),
	), copyTransportObjectsHandler(client, fallback))

	s.AddTool(mcp.NewTool("add_package_to_transport",
		mcp.WithTitleAnnotation("Add Package to Transport"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Add the objects of a package to a transport request, optionally with its subpackages. "+
				"The usual target is a transport of copies (create_transport with category T). "+
				verifyCopiesHint+copiesFallbackHint,
		),
		mcp.WithString("package", mcp.Required(), mcp.Description("Package whose objects are added, e.g. ZMY_PACKAGE")),
		mcp.WithString("target_request", mcp.Required(), mcp.Description("Modifiable request that receives the entries, e.g. DEVK900124")),
		mcp.WithBoolean("include_subpackages", mcp.Description("Also add the objects of all subpackages (default: false)")),
		mcp.WithOutputSchema[AddPackageToTransportResult](),
	), addPackageToTransportHandler(client, fallback))
}

func copyTransportObjectsHandler(client adt.TransportClient, fallback BlackMagicClient) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source, errRes := requireString(req, "source_request")
		if errRes != nil {
			return errRes, nil
		}
		target, errRes := requireString(req, "target_request")
		if errRes != nil {
			return errRes, nil
		}
		withDoc := req.GetBool("with_documentation", true)
		source, target = normalizeName(source), normalizeName(target)
		res := CopyTransportObjectsResult{SourceRequest: source, TargetRequest: target, Copied: true}
		err := client.CopyTransportObjects(ctx, source, target, withDoc)
		if fb, ok := copiesFallbackFor(err, fallback); ok {
			if fbErr := fb.CopyTransportObjectsFallback(ctx, source, target, withDoc); fbErr != nil {
				return errorResult(fmt.Errorf("ADT cannot copy the entries here and the fallback failed: %w (ADT: %v)", fbErr, err)), nil
			}
			res.ViaFallback = true
			return mcp.NewToolResultJSON(res)
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	}
}

func addPackageToTransportHandler(client adt.TransportClient, fallback BlackMagicClient) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkg, errRes := requireString(req, "package")
		if errRes != nil {
			return errRes, nil
		}
		target, errRes := requireString(req, "target_request")
		if errRes != nil {
			return errRes, nil
		}
		sub := req.GetBool("include_subpackages", false)
		pkg, target = normalizeName(pkg), normalizeName(target)
		res := AddPackageToTransportResult{Package: pkg, TargetRequest: target, IncludeSubpackages: sub, Added: true}
		err := client.AddPackageToTransport(ctx, pkg, target, sub)
		if fb, ok := copiesFallbackFor(err, fallback); ok {
			if fbErr := fb.AddPackageToTransportFallback(ctx, pkg, target, sub); fbErr != nil {
				return errorResult(fmt.Errorf("ADT cannot add the package here and the fallback failed: %w (ADT: %v)", fbErr, err)), nil
			}
			res.ViaFallback = true
			return mcp.NewToolResultJSON(res)
		}
		if err != nil {
			return errorResult(err), nil
		}
		return mcp.NewToolResultJSON(res)
	}
}

// normalizeName upper-cases and trims a request or package name, as adtler
// does before sending it, so the result and the fallback see the same value.
func normalizeName(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// copiesFallbackFor returns the configured TransportCopiesFallback when err
// is adtler's ErrorNotSupported, the only case the fallback is for (#602).
func copiesFallbackFor(err error, fallback BlackMagicClient) (TransportCopiesFallback, bool) {
	if err == nil || adt.ClassifyError(err) != adt.ErrorNotSupported {
		return nil, false
	}
	fb, ok := fallback.(TransportCopiesFallback)
	return fb, ok
}
