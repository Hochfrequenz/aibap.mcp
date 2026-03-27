package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func (c *httpClient) ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error) {
	objects := make([]adtxml.ActivationObject, len(objectURIs))
	for i, uri := range objectURIs {
		objects[i] = adtxml.ActivationObject{URI: uri}
	}
	bodyXML, err := xml.Marshal(adtxml.ActivationRequest{
		NS:      nsADTCore,
		Objects: objects,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal activation request: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/activation?method=activate&preauditRequested=true",
		strings.NewReader(xml.Header+string(bodyXML)),
		map[string]string{
			"Content-Type": contentTypeXML,
			"Accept":       "application/xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("ActivateObjects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var msgs adtxml.ActivationMessages
	xml.Unmarshal(data, &msgs) //nolint:errcheck

	result := &ActivationResult{Success: true}
	for _, m := range msgs.Messages {
		msg := ActivationMessage{
			ObjectURI: m.Href,
			Type:      m.Type,
			Text:      m.ShortText.Text,
		}
		result.Messages = append(result.Messages, msg)
		if m.Type == "E" {
			result.Success = false
		}
	}
	return result, nil
}

func (c *httpClient) GetInactiveObjects(ctx context.Context) ([]ObjectInfo, error) {
	resp, err := c.doRead(ctx, "/sap/bc/adt/activation/inactiveobjects",
		map[string]string{"Accept": "application/vnd.sap.adt.inactivectsobjects.v1+xml"})
	if err != nil {
		return nil, fmt.Errorf("GetInactiveObjects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetInactiveObjects reading body: %w", err)
	}

	var root struct {
		Objects []struct {
			URI         string `xml:"uri,attr"`
			Type        string `xml:"type,attr"`
			Name        string `xml:"name,attr"`
			Description string `xml:"description,attr"`
			ParentURI   string `xml:"parentUri,attr"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("GetInactiveObjects parsing: %w", err)
	}

	result := make([]ObjectInfo, len(root.Objects))
	for i, o := range root.Objects {
		result[i] = ObjectInfo{
			URI:         o.URI,
			Type:        o.Type,
			Name:        o.Name,
			Description: o.Description,
		}
	}
	return result, nil
}
