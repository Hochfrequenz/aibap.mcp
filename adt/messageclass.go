package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// MessageClassInfo describes a message class with its messages.
type MessageClassInfo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Package     string    `json:"package"`
	Messages    []Message `json:"messages"`
}

// Message is a single message entry in a message class.
type Message struct {
	Number   string `json:"number"`
	Text     string `json:"text"`
	SelfExpl bool   `json:"self_explanatory"`
}

// GetMessageClass reads all messages of a message class (e.g. "00", "ZFOO").
func (c *httpClient) GetMessageClass(ctx context.Context, messageClassName string) (*MessageClassInfo, error) {
	path := "/sap/bc/adt/messageclass/" + strings.ToLower(messageClassName)
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetMessageClass: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetMessageClass: reading body: %w", err)
	}

	return parseMessageClassXML(data)
}

// MessageSearchResult is a single entry from the message search endpoint.
type MessageSearchResult struct {
	Name        string `json:"name"`        // "MSGCLASS/NNN"
	Description string `json:"description"` // message text
	URI         string `json:"uri"`         // message class URI
}

// SearchMessages searches for messages across all message classes using the
// ADT /sap/bc/adt/repository/informationsystem/messagesearch endpoint.
// The query is a type-ahead filter on the message class ID (e.g. "00", "Z*").
func (c *httpClient) SearchMessages(ctx context.Context, query string, maxResults int) ([]MessageSearchResult, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	path := fmt.Sprintf("/sap/bc/adt/repository/informationsystem/messagesearch?name=%s&maxItemCount=%d",
		query, maxResults)
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("SearchMessages: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("SearchMessages: reading body: %w", err)
	}

	return parseNamedItemList(data)
}

func parseNamedItemList(data []byte) ([]MessageSearchResult, error) {
	var doc struct {
		Items []struct {
			Name        string `xml:"name"`
			Description string `xml:"description"`
			Data        string `xml:"data"`
		} `xml:"namedItem"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing named item list: %w", err)
	}

	var results []MessageSearchResult
	for _, item := range doc.Items {
		results = append(results, MessageSearchResult{
			Name:        item.Name,
			Description: item.Description,
			URI:         item.Data,
		})
	}
	return results, nil
}

func parseMessageClassXML(data []byte) (*MessageClassInfo, error) {
	var doc struct {
		XMLName     xml.Name `xml:"messageClass"`
		Name        string   `xml:"name,attr"`
		Description string   `xml:"description,attr"`
		PackageRef  struct {
			Name string `xml:"name,attr"`
		} `xml:"packageRef"`
		Messages []struct {
			Number   string `xml:"msgno,attr"`
			Text     string `xml:"msgtext,attr"`
			SelfExpl string `xml:"selfexplainatory,attr"`
		} `xml:"messages"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing message class XML: %w", err)
	}

	result := &MessageClassInfo{
		Name:        doc.Name,
		Description: doc.Description,
		Package:     doc.PackageRef.Name,
	}
	for _, m := range doc.Messages {
		if m.Text == "" && m.Number == "" {
			continue
		}
		result.Messages = append(result.Messages, Message{
			Number:   m.Number,
			Text:     m.Text,
			SelfExpl: m.SelfExpl == "true",
		})
	}
	return result, nil
}
