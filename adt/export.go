package adt

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// ExportPackage exports an ABAP package as an abapGit-compatible ZIP file.
//
// This calls the custom ADT endpoint /sap/bc/adt/abapgit/export/packages
// which wraps ZCL_ABAPGIT_ZIP=>EXPORT on the SAP side. The endpoint is
// provided by the companion ABAP package:
// https://github.com/Hochfrequenz/Z_ABABGIT_ADT_EXPORT
//
// Returns the raw ZIP bytes. Returns an error if the package does not exist
// or if serialization fails.
func (c *httpClient) ExportPackage(ctx context.Context, packageName string) ([]byte, error) {
	pkg := strings.ToUpper(strings.TrimSpace(packageName))
	if pkg == "" {
		return nil, fmt.Errorf("ExportPackage: package name must not be empty")
	}

	params := url.Values{}
	params.Set("package", pkg)
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
