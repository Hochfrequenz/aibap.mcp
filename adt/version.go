package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// VersionInfo describes a single version of an ABAP source object.
type VersionInfo struct {
	VersionNumber string `json:"version_number"`
	Author        string `json:"author"`
	Date          string `json:"date"`      // ISO 8601 UTC
	Transport     string `json:"transport"` // transport number (empty for local)
	TransportDesc string `json:"transport_description"`
	ContentURI    string `json:"content_uri"` // URI to fetch the source of this version
}

// GetVersionHistory returns the version history of an ABAP source object.
// Uses the ADT /source/main/versions endpoint which returns an Atom feed.
func (c *httpClient) GetVersionHistory(ctx context.Context, objectURI string) ([]VersionInfo, error) {
	path := objectURI + "/source/main/versions"
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/atom+xml"})
	if err != nil {
		return nil, fmt.Errorf("GetVersionHistory: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetVersionHistory reading body: %w", err)
	}

	return parseVersionFeed(data)
}

func parseVersionFeed(data []byte) ([]VersionInfo, error) {
	var feed struct {
		Entries []struct {
			ID      string `xml:"id"`
			Updated string `xml:"updated"`
			Title   string `xml:"title"`
			Authors []struct {
				Name string `xml:"name"`
			} `xml:"author"`
			Content struct {
				Src string `xml:"src,attr"`
			} `xml:"content"`
			Links []struct {
				Rel  string `xml:"rel,attr"`
				Href string `xml:"href,attr"`
				Name string `xml:"name,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing version feed: %w", err)
	}

	var versions []VersionInfo
	for _, e := range feed.Entries {
		v := VersionInfo{
			VersionNumber: e.ID,
			Date:          e.Updated,
			ContentURI:    e.Content.Src,
		}
		if len(e.Authors) > 0 {
			v.Author = e.Authors[0].Name
		}
		// Transport info is in links with rel containing "trans_req"
		for _, l := range e.Links {
			if strings.Contains(l.Rel, "trans") {
				v.Transport = l.Name
				v.TransportDesc = l.Href
			}
		}
		if v.Transport == "" && e.Title != "" {
			v.TransportDesc = e.Title
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// GetVersionSource returns the source code of a specific historical version.
// The contentURI comes from GetVersionHistory (VersionInfo.ContentURI).
func (c *httpClient) GetVersionSource(ctx context.Context, contentURI string) (string, error) {
	resp, err := c.doRead(ctx, contentURI, map[string]string{"Accept": "text/plain"})
	if err != nil {
		return "", fmt.Errorf("GetVersionSource: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("GetVersionSource reading body: %w", err)
	}
	return string(data), nil
}

// DiffResult holds the result of comparing active vs inactive source.
type DiffResult struct {
	HasChanges bool   `json:"has_changes"`
	Active     string `json:"active"`
	Inactive   string `json:"inactive"`
}

// DiffActiveInactive compares the active (last activated) and inactive (saved
// but not activated) source of an object.
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
