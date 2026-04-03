package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BAdIDefinition describes a BAdI definition within an enhancement spot.
type BAdIDefinition struct {
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	SingleUse        bool         `json:"single_use"`
	UseFallbackClass bool         `json:"use_fallback_class"`
	Interface        ObjectRef    `json:"interface"`
	SampleClass      *ObjectRef   `json:"sample_class,omitempty"`
	Filters          []BAdIFilter `json:"filters,omitempty"`
}

// BAdIFilter is a filter definition on a BAdI.
type BAdIFilter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ObjectRef is a reference to an ABAP object with URI, type and name.
type ObjectRef struct {
	URI  string `json:"uri"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// EnhancementSpotInfo describes an enhancement spot (BAdI definitions).
type EnhancementSpotInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Package     string           `json:"package"`
	Definitions []BAdIDefinition `json:"definitions"`
}

// BAdIImplementationInfo describes an enhancement implementation.
type BAdIImplementationInfo struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Package         string          `json:"package"`
	ETag            string          `json:"etag,omitempty"`
	RawXML          string          `json:"-"` // full XML for round-trip PUT
	Implementations []BAdIImplEntry `json:"implementations"`
}

// BAdIImplEntry is a single BAdI implementation within an enhancement implementation.
type BAdIImplEntry struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	IsActive          bool      `json:"is_active"`
	IsDefault         bool      `json:"is_default"`
	EnhancementSpot   ObjectRef `json:"enhancement_spot"`
	BAdIDefinition    ObjectRef `json:"badi_definition"`
	ImplementingClass ObjectRef `json:"implementing_class"`
}

// GetEnhancementSpot reads a BAdI enhancement spot definition.
func (c *httpClient) GetEnhancementSpot(ctx context.Context, spotName string) (*EnhancementSpotInfo, error) {
	path := "/sap/bc/adt/enhancements/enhsxs/" + spotName
	resp, err := c.doRead(ctx, path, map[string]string{
		"Accept": "application/vnd.sap.adt.enh.enhs.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("GetEnhancementSpot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetEnhancementSpot: reading body: %w", err)
	}
	return parseEnhancementSpotXML(data)
}

const enhoContentType = "application/vnd.sap.adt.enh.enho.v1+xml"

// GetEnhancementImplementation reads a BAdI enhancement implementation.
func (c *httpClient) GetEnhancementImplementation(ctx context.Context, implName string) (*BAdIImplementationInfo, error) {
	path := "/sap/bc/adt/enhancements/enhoxh/" + implName
	resp, err := c.doRead(ctx, path, map[string]string{
		"Accept": enhoContentType,
	})
	if err != nil {
		return nil, fmt.Errorf("GetEnhancementImplementation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetEnhancementImplementation: reading body: %w", err)
	}
	result, err := parseEnhancementImplXML(data)
	if err != nil {
		return nil, err
	}
	result.ETag = resp.Header.Get("ETag")
	result.RawXML = string(data)
	return result, nil
}

// SetEnhancementImplementation writes the full XML of a BAdI enhancement implementation.
// Use GetEnhancementImplementation to get the RawXML, modify it, then pass it back.
func (c *httpClient) SetEnhancementImplementation(ctx context.Context, implName, xmlBody, lockHandle, transport, etag string) error {
	path := "/sap/bc/adt/enhancements/enhoxh/" + implName
	headers := map[string]string{
		"Content-Type": enhoContentType,
	}
	if etag != "" {
		headers["If-Match"] = etag
	}
	if lockHandle != "" {
		headers["X-SAP-Lock-Handle"] = lockHandle
	}
	if transport != "" {
		path += "?corrNr=" + transport
	}
	resp, err := c.doMutate(ctx, http.MethodPut, path,
		strings.NewReader(xmlBody),
		headers,
	)
	if err != nil {
		return fmt.Errorf("SetEnhancementImplementation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

func parseEnhancementSpotXML(data []byte) (*EnhancementSpotInfo, error) {
	var doc struct {
		XMLName     xml.Name `xml:"objectData"`
		Name        string   `xml:"name,attr"`
		Description string   `xml:"description,attr"`
		PackageRef  struct {
			Name string `xml:"name,attr"`
		} `xml:"packageRef"`
		ContentSpecific struct {
			BAdIDefs []struct {
				Name             string `xml:"name,attr"`
				Description      string `xml:"shorttext,attr"`
				SingleUse        string `xml:"singleUse,attr"`
				UseFallbackClass string `xml:"useFallbackClass,attr"`
				Interface        struct {
					URI  string `xml:"uri,attr"`
					Type string `xml:"type,attr"`
					Name string `xml:"name,attr"`
				} `xml:"interface"`
				SampleClasses struct {
					Class struct {
						URI  string `xml:"uri,attr"`
						Type string `xml:"type,attr"`
						Name string `xml:"name,attr"`
					} `xml:"sampleClass"`
				} `xml:"sampleClasses"`
				Filters struct {
					Items []struct {
						Name string `xml:"filterName,attr"`
						Type string `xml:"filterType,attr"`
					} `xml:"filter"`
				} `xml:"filters"`
			} `xml:"badiDefinition"`
		} `xml:"contentSpecific>badiTechnology>badiDefinitions"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing enhancement spot XML: %w", err)
	}

	result := &EnhancementSpotInfo{
		Name:        doc.Name,
		Description: doc.Description,
		Package:     doc.PackageRef.Name,
	}
	for _, bd := range doc.ContentSpecific.BAdIDefs {
		def := BAdIDefinition{
			Name:             bd.Name,
			Description:      bd.Description,
			SingleUse:        bd.SingleUse == xmlTrue,
			UseFallbackClass: bd.UseFallbackClass == xmlTrue,
			Interface:        ObjectRef{URI: bd.Interface.URI, Type: bd.Interface.Type, Name: bd.Interface.Name},
		}
		if bd.SampleClasses.Class.Name != "" {
			def.SampleClass = &ObjectRef{URI: bd.SampleClasses.Class.URI, Type: bd.SampleClasses.Class.Type, Name: bd.SampleClasses.Class.Name}
		}
		for _, f := range bd.Filters.Items {
			def.Filters = append(def.Filters, BAdIFilter{Name: f.Name, Type: f.Type})
		}
		result.Definitions = append(result.Definitions, def)
	}
	return result, nil
}

func parseEnhancementImplXML(data []byte) (*BAdIImplementationInfo, error) {
	var doc struct {
		XMLName     xml.Name `xml:"objectData"`
		Name        string   `xml:"name,attr"`
		Description string   `xml:"description,attr"`
		PackageRef  struct {
			Name string `xml:"name,attr"`
		} `xml:"packageRef"`
		ContentSpecific struct {
			BAdIImpls []struct {
				Name        string `xml:"name,attr"`
				Description string `xml:"shortText,attr"`
				IsActive    string `xml:"isActive,attr"`
				IsDefault   string `xml:"isDefault,attr"`
				EnhSpot     struct {
					URI  string `xml:"uri,attr"`
					Type string `xml:"type,attr"`
					Name string `xml:"name,attr"`
				} `xml:"enhancementSpot"`
				BAdIDef struct {
					URI  string `xml:"uri,attr"`
					Type string `xml:"type,attr"`
					Name string `xml:"name,attr"`
				} `xml:"badiDefinition"`
				ImplClass struct {
					URI  string `xml:"uri,attr"`
					Type string `xml:"type,attr"`
					Name string `xml:"name,attr"`
				} `xml:"implementingClass"`
			} `xml:"badiImplementation"`
		} `xml:"contentSpecific>badiTechnology>badiImplementations"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing enhancement implementation XML: %w", err)
	}

	result := &BAdIImplementationInfo{
		Name:        doc.Name,
		Description: doc.Description,
		Package:     doc.PackageRef.Name,
	}
	for _, bi := range doc.ContentSpecific.BAdIImpls {
		result.Implementations = append(result.Implementations, BAdIImplEntry{
			Name:              bi.Name,
			Description:       bi.Description,
			IsActive:          bi.IsActive == xmlTrue,
			IsDefault:         bi.IsDefault == xmlTrue,
			EnhancementSpot:   ObjectRef{URI: bi.EnhSpot.URI, Type: bi.EnhSpot.Type, Name: bi.EnhSpot.Name},
			BAdIDefinition:    ObjectRef{URI: bi.BAdIDef.URI, Type: bi.BAdIDef.Type, Name: bi.BAdIDef.Name},
			ImplementingClass: ObjectRef{URI: bi.ImplClass.URI, Type: bi.ImplClass.Type, Name: bi.ImplClass.Name},
		})
	}
	return result, nil
}
