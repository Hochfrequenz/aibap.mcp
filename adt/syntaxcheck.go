package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adtmodel"
)

func (c *httpClient) SyntaxCheck(ctx context.Context, objectURI string) ([]SyntaxMessage, error) {
	params := url.Values{}
	params.Set("adtObjectUri", objectURI)

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/checkruns?"+params.Encode(),
		strings.NewReader(""),
		map[string]string{
			"Content-Type": contentTypeXML,
			"Accept":       contentTypeXML,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("SyntaxCheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var msgs adtmodel.CheckMessages
	xml.Unmarshal(data, &msgs) //nolint:errcheck

	result := make([]SyntaxMessage, len(msgs.Messages))
	for i, m := range msgs.Messages {
		result[i] = SyntaxMessage{
			Type:   m.Type,
			Text:   m.ShortText.Text,
			Line:   m.Line,
			Column: m.Column,
		}
	}
	return result, nil
}
