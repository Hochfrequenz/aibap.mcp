package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type xmlActivationRequest struct {
	XMLName xml.Name `xml:"adtcore:objectReferences"`
	NS      string   `xml:"xmlns:adtcore,attr"`
	Objects []xmlActivationObject
}

type xmlActivationObject struct {
	XMLName xml.Name `xml:"adtcore:objectReference"`
	URI     string   `xml:"adtcore:uri,attr"`
}

type xmlActivationMessages struct {
	XMLName  xml.Name               `xml:"messages"`
	Messages []xmlActivationMessage `xml:"message"`
}

type xmlActivationMessage struct {
	URI       string `xml:"uri,attr"`
	Type      string `xml:"type,attr"`
	ShortText struct {
		Text string `xml:"shortText"`
	} `xml:"shortTextElements"`
}

func (c *httpClient) ActivateObjects(ctx context.Context, objectURIs []string) (*ActivationResult, error) {
	objects := make([]xmlActivationObject, len(objectURIs))
	for i, uri := range objectURIs {
		objects[i] = xmlActivationObject{URI: uri}
	}
	bodyXML, err := xml.Marshal(xmlActivationRequest{
		NS:      nsADTCore,
		Objects: objects,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal activation request: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/activation?method=activate&preauditRequested=true",
		strings.NewReader(xml.Header+string(bodyXML)),
		map[string]string{"Content-Type": contentTypeXML},
	)
	if err != nil {
		return nil, fmt.Errorf("ActivateObjects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var msgs xmlActivationMessages
	xml.Unmarshal(data, &msgs) //nolint:errcheck

	result := &ActivationResult{Success: true}
	for _, m := range msgs.Messages {
		msg := ActivationMessage{
			ObjectURI: m.URI,
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
