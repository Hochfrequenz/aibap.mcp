package adt

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// folderLogicFullHint is the error substring from abapGit's serializer when PREFIX
// folder logic fails due to non-conforming sub-package names. Originates from
// zcl_abapgit_zip=>export via cx_root->get_text(). May break if abapGit changes
// the message wording or on non-English SAP systems. See issue for structured error:
// https://github.com/Hochfrequenz/mcp-server-abap/issues/102
const folderLogicFullHint = "folder logic FULL"

// ExportPackage exports an ABAP package as an abapGit-compatible ZIP file.
//
// This calls the custom ADT endpoint /sap/bc/adt/abapgit/export/packages
// which wraps ZCL_ABAPGIT_ZIP=>EXPORT on the SAP side. The endpoint is
// provided by the companion ABAP package:
// https://github.com/Hochfrequenz/Z_ABABGIT_ADT_EXPORT
//
// Uses PREFIX folder logic by default. If the package has sub-packages that
// don't follow PREFIX naming, the endpoint returns an error containing
// "Try using the folder logic FULL". In that case, the request is automatically
// retried with folderLogic=FULL.
//
// Returns the raw ZIP bytes. Returns an error if the package does not exist
// or if serialization fails.
func (c *httpClient) ExportPackage(ctx context.Context, packageName string) ([]byte, error) {
	pkg := strings.ToUpper(strings.TrimSpace(packageName))
	if pkg == "" {
		return nil, fmt.Errorf("ExportPackage: package name must not be empty")
	}

	data, err := c.exportPackageWithFolderLogic(ctx, pkg, "")
	if err != nil && strings.Contains(err.Error(), folderLogicFullHint) {
		// Retry with FULL folder logic for packages with non-PREFIX sub-packages.
		data, err = c.exportPackageWithFolderLogic(ctx, pkg, "FULL")
	}
	return data, err
}

func (c *httpClient) exportPackageWithFolderLogic(ctx context.Context, pkg, folderLogic string) ([]byte, error) {
	params := url.Values{}
	params.Set("package", pkg)
	if folderLogic != "" {
		params.Set("folderLogic", folderLogic)
	}
	path := "/sap/bc/adt/abapgit/export/packages?" + params.Encode()

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/zip"})
	if err != nil {
		return nil, fmt.Errorf("ExportPackage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ExportPackage: reading response: %w", err)
	}
	return data, nil
}
