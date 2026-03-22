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
	XMLName xml.Name              `xml:"adtcore:objectReferences"`
	NS      string                `xml:"xmlns:adtcore,attr"`
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

func (c *httpClient) ActivateObject(ctx context.Context, objectURI string) (*ActivationResult, error) {
	bodyXML, err := xml.Marshal(xmlActivationRequest{
		NS:      "http://www.sap.com/adt/core",
		Objects: []xmlActivationObject{{URI: objectURI}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal activation request: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/activation/activate?method=activate&preauditRequested=true",
		strings.NewReader(xml.Header+string(bodyXML)),
		map[string]string{"Content-Type": "application/xml"},
	)
	if err != nil {
		return nil, fmt.Errorf("ActivateObject: %w", err)
	}
	defer resp.Body.Close()

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
