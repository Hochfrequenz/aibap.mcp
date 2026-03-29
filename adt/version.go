package adt

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// VersionInfo describes a single version entry from VRSD.
type VersionInfo struct {
	VersionNumber string `json:"version_number"`
	Author        string `json:"author"`
	Date          string `json:"date"` // YYYYMMDD
	Time          string `json:"time"` // HHMMSS
	Transport     string `json:"transport"`
}

// GetVersionHistory returns the version history of an ABAP object from VRSD.
// objName is the ABAP object name (e.g. Z_MY_REPORT).
// objType is the VRSD object type (e.g. REPS for reports, CLSD for class definitions,
// METH for methods). Use REPS for programs.
func (c *httpClient) GetVersionHistory(ctx context.Context, objName, objType string) ([]VersionInfo, error) {
	name := strings.ToUpper(strings.TrimSpace(objName))
	typ := strings.ToUpper(strings.TrimSpace(objType))
	if name == "" || typ == "" {
		return nil, fmt.Errorf("GetVersionHistory: objName and objType must not be empty")
	}

	sql := fmt.Sprintf(
		"SELECT VERSNO, AUTHOR, DATUM, ZEIT, KORRNUM FROM VRSD "+
			"WHERE OBJNAME = '%s' AND OBJTYPE = '%s' ORDER BY VERSNO DESCENDING",
		strings.ReplaceAll(name, "'", "''"),
		strings.ReplaceAll(typ, "'", "''"),
	)
	result, err := c.RunQuery(ctx, sql, 100)
	if err != nil {
		return nil, fmt.Errorf("GetVersionHistory: %w", err)
	}

	var versions []VersionInfo
	for _, row := range result.Rows {
		if len(row) < 5 {
			continue
		}
		versions = append(versions, VersionInfo{
			VersionNumber: strings.TrimSpace(row[0]),
			Author:        strings.TrimSpace(row[1]),
			Date:          strings.TrimSpace(row[2]),
			Time:          strings.TrimSpace(row[3]),
			Transport:     strings.TrimSpace(row[4]),
		})
	}
	return versions, nil
}

// DiffResult holds the result of comparing active vs inactive source.
type DiffResult struct {
	HasChanges bool   `json:"has_changes"`
	Active     string `json:"active"`
	Inactive   string `json:"inactive"`
}

// DiffActiveInactive compares the active (last activated) and inactive (saved
// but not activated) source of an object. Returns both versions so the caller
// can compute a diff.
func (c *httpClient) DiffActiveInactive(ctx context.Context, objectURI string) (*DiffResult, error) {
	activeSrc, err := c.getSourceWithVersion(ctx, objectURI, "active")
	if err != nil {
		return nil, fmt.Errorf("DiffActiveInactive active: %w", err)
	}
	inactiveSrc, err := c.getSourceWithVersion(ctx, objectURI, "inactive")
	if err != nil {
		return nil, fmt.Errorf("DiffActiveInactive inactive: %w", err)
	}

	return &DiffResult{
		HasChanges: activeSrc != inactiveSrc,
		Active:     activeSrc,
		Inactive:   inactiveSrc,
	}, nil
}

func (c *httpClient) getSourceWithVersion(ctx context.Context, objectURI, version string) (string, error) {
	path := objectURI + "/source/main?version=" + version
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "text/plain"})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
