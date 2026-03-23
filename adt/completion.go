package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type xmlCompletions struct {
	XMLName xml.Name        `xml:"completions"`
	Items   []xmlCompletion `xml:"completion"`
}

type xmlCompletion struct {
	Text        string `xml:"text,attr"`
	Description string `xml:"description,attr"`
}

func (c *httpClient) GetCompletions(ctx context.Context, objectURI, source string, line, column int) ([]CompletionItem, error) {
	params := url.Values{}
	params.Set("uri", objectURI+"/source/main")
	params.Set("line", strconv.Itoa(line))
	params.Set("column", strconv.Itoa(column))
	path := "/sap/bc/adt/abapsource/codecompletion/proposals?" + params.Encode()

	resp, err := c.doMutate(ctx, "POST", path,
		strings.NewReader(source),
		map[string]string{
			"Content-Type": "text/plain; charset=utf-8",
			"Accept":       contentTypeXML,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("GetCompletions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetCompletions reading body: %w", err)
	}
	var comps xmlCompletions
	if err := xml.Unmarshal(data, &comps); err != nil {
		return nil, fmt.Errorf("GetCompletions parsing: %w", err)
	}
	result := make([]CompletionItem, len(comps.Items))
	for i, c := range comps.Items {
		result[i] = CompletionItem(c)
	}
	return result, nil
}
