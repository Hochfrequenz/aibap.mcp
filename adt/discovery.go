package adt

import (
	"context"
	"encoding/xml"
	"strings"
)

// parseDiscovery extracts accepted content types per endpoint from the ADT
// discovery XML (/sap/bc/adt/discovery). The result maps endpoint paths
// (e.g. "/sap/bc/adt/packages") to their accepted content types
// (e.g. ["application/vnd.sap.adt.packages.v2+xml", "...v1+xml"]).
func parseDiscovery(data []byte) map[string][]string {
	var doc struct {
		Workspaces []struct {
			Collections []struct {
				Href    string   `xml:"href,attr"`
				Accepts []string `xml:"accept"`
			} `xml:"collection"`
		} `xml:"workspace"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil
	}

	result := make(map[string][]string)
	for _, ws := range doc.Workspaces {
		for _, c := range ws.Collections {
			if c.Href != "" && len(c.Accepts) > 0 {
				result[c.Href] = c.Accepts
			}
		}
	}
	return result
}

// hasEndpointInDiscovery checks whether the discovery cache contains an entry
// whose href starts with the given prefix. Triggers a CSRF fetch (which
// populates the cache) if no discovery data is available yet.
func (c *httpClient) hasEndpointInDiscovery(ctx context.Context, pathPrefix string) bool {
	c.mu.Lock()
	if c.discovery == nil && c.csrfToken == "" {
		_ = c.fetchCSRFToken(ctx)
	}
	defer c.mu.Unlock()
	for endpoint := range c.discovery {
		if endpoint == pathPrefix || strings.HasPrefix(endpoint, pathPrefix) {
			return true
		}
	}
	return false
}

// NegotiateContentType returns the best content type for the given endpoint
// based on the cached discovery data. It prefers the first match from the
// preferred list, falling back to the default if no match is found.
//
// This allows the client to automatically use the correct v1/v2 content type
// based on what the SAP system actually supports.
func (c *httpClient) NegotiateContentType(endpoint string, preferred []string, defaultCT string) string {
	c.mu.Lock()
	accepted := c.discovery[endpoint]
	c.mu.Unlock()

	if len(accepted) == 0 {
		return defaultCT
	}

	// Build a set for fast lookup
	acceptedSet := make(map[string]bool, len(accepted))
	for _, a := range accepted {
		acceptedSet[a] = true
	}

	// Return the first preferred type that the server accepts
	for _, p := range preferred {
		if acceptedSet[p] {
			return p
		}
	}
	return defaultCT
}
