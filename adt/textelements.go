package adt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TextElements contains the text symbols and selection texts of an ABAP object.
type TextElements struct {
	Symbols    []TextSymbol    `json:"symbols,omitempty"`
	Selections []SelectionText `json:"selections,omitempty"`
}

// TextSymbol is a single text element (TEXT-001, TEXT-002...).
type TextSymbol struct {
	Key       string `json:"key"`
	Text      string `json:"text"`
	MaxLength int    `json:"max_length,omitempty"`
}

// SelectionText is a selection screen text for a parameter or select-option.
type SelectionText struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// textElementEndpoints maps object type prefixes to their text element endpoint paths.
var textElementEndpoints = map[string]string{
	"/sap/bc/adt/programs/programs/": "/sap/bc/adt/textelements/programs/",
	"/sap/bc/adt/oo/classes/":        "/sap/bc/adt/textelements/classes/",
	"/sap/bc/adt/functions/groups/":  "/sap/bc/adt/textelements/functiongroups/",
}

// GetTextElements reads text symbols and selection texts for an ABAP object.
// The objectURI must be a program, class, or function group URI.
func (c *httpClient) GetTextElements(ctx context.Context, objectURI string) (*TextElements, error) {
	basePath, err := resolveTextElementPath(objectURI)
	if err != nil {
		return nil, err
	}

	result := &TextElements{}
	var lastErr error

	// Read text symbols
	symbols, err := c.readTextElementSource(ctx, basePath+"/source/symbols",
		"application/vnd.sap.adt.textelements.symbols.v1")
	if err == nil {
		result.Symbols = parseTextSymbols(symbols)
	} else {
		lastErr = err
	}

	// Read selection texts
	selections, err := c.readTextElementSource(ctx, basePath+"/source/selections",
		"application/vnd.sap.adt.textelements.selections.v1")
	if err == nil {
		result.Selections = parseSelectionTexts(selections)
	} else {
		lastErr = err
	}

	// If both failed, report the error (endpoint likely not available)
	if result.Symbols == nil && result.Selections == nil && lastErr != nil {
		return nil, lastErr
	}

	return result, nil
}

// SetTextElements writes text symbols and/or selection texts for an ABAP object.
// At least one of symbols or selections must be non-nil.
// The lockHandle and transport are passed as query parameters (not headers).
func (c *httpClient) SetTextElements(ctx context.Context, objectURI string, symbols []TextSymbol, selections []SelectionText, lockHandle, transport string) error {
	basePath, err := resolveTextElementPath(objectURI)
	if err != nil {
		return err
	}

	if symbols != nil {
		if err := c.writeTextElementSource(ctx, basePath+"/source/symbols",
			"application/vnd.sap.adt.textelements.symbols.v1",
			formatTextSymbols(symbols), lockHandle, transport); err != nil {
			return fmt.Errorf("SetTextElements symbols: %w", err)
		}
	}

	if selections != nil {
		if err := c.writeTextElementSource(ctx, basePath+"/source/selections",
			"application/vnd.sap.adt.textelements.selections.v1",
			formatSelectionTexts(selections), lockHandle, transport); err != nil {
			return fmt.Errorf("SetTextElements selections: %w", err)
		}
	}

	return nil
}

func (c *httpClient) writeTextElementSource(ctx context.Context, path, contentType, body, lockHandle, transport string) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	if lockHandle != "" {
		path += sep + "lockHandle=" + lockHandle
		sep = "&"
	}
	if transport != "" {
		path += sep + "corrNr=" + transport
	}

	resp, err := c.doMutate(ctx, http.MethodPut, path, strings.NewReader(body),
		map[string]string{
			"Content-Type": contentType,
		},
	)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

func resolveTextElementPath(objectURI string) (string, error) {
	upper := strings.ToUpper(objectURI)
	for prefix, tePath := range textElementEndpoints {
		upperPrefix := strings.ToUpper(prefix)
		if strings.HasPrefix(upper, upperPrefix) {
			name := objectURI[len(prefix):]
			return tePath + name, nil
		}
	}
	return "", fmt.Errorf("GetTextElements: unsupported object type for URI %q (only programs, classes, function groups)", objectURI)
}

func (c *httpClient) readTextElementSource(ctx context.Context, path, accept string) (string, error) {
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": accept})
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

// parseTextSymbols parses the text symbols format:
//
//	@MaxLength:50
//	001=ABAP Objects Performance Examples
func parseTextSymbols(source string) []TextSymbol {
	var symbols []TextSymbol
	maxLen := 0
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@MaxLength:") {
			fmt.Sscanf(line, "@MaxLength:%d", &maxLen) //nolint:errcheck
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			symbols = append(symbols, TextSymbol{
				Key:       strings.TrimSpace(line[:idx]),
				Text:      line[idx+1:],
				MaxLength: maxLen,
			})
			maxLen = 0
		}
	}
	return symbols
}

// parseSelectionTexts parses the selection texts format:
//
//	OSCLOCK =Betriebssystemuhr
func parseSelectionTexts(source string) []SelectionText {
	var texts []SelectionText
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			texts = append(texts, SelectionText{
				Name: strings.TrimSpace(line[:idx]),
				Text: line[idx+1:],
			})
		}
	}
	return texts
}

// formatTextSymbols builds the plaintext body for PUT text symbols.
func formatTextSymbols(symbols []TextSymbol) string {
	var b strings.Builder
	for _, s := range symbols {
		if s.MaxLength > 0 {
			fmt.Fprintf(&b, "@MaxLength:%d\n", s.MaxLength)
		}
		fmt.Fprintf(&b, "%s=%s\n", s.Key, s.Text)
	}
	return b.String()
}

// formatSelectionTexts builds the plaintext body for PUT selection texts.
func formatSelectionTexts(selections []SelectionText) string {
	var b strings.Builder
	for _, s := range selections {
		fmt.Fprintf(&b, "%-8s=%s\n", s.Name, s.Text)
	}
	return b.String()
}
