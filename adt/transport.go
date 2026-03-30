package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Hochfrequenz/mcp-server-abap/adt/adtxml"
)

func (c *httpClient) CheckTransport(ctx context.Context, pgmID, object, objectName string) (*TransportCheckResult, error) {
	reqData := adtxml.TransportCheckRequest{
		PgmID:      strings.ToUpper(pgmID),
		Object:     strings.ToUpper(object),
		ObjectName: strings.ToUpper(objectName),
		Operation:  "I",
	}
	body, err := adtxml.MarshalASXData(reqData)
	if err != nil {
		return nil, fmt.Errorf("CheckTransport marshal: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/cts/transportchecks",
		strings.NewReader(string(body)),
		map[string]string{
			"Content-Type": "application/vnd.sap.as+xml; charset=utf-8; dataname=com.sap.adt.transport.CheckObjects",
			"Accept":       "application/vnd.sap.as+xml",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("CheckTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("CheckTransport reading body: %w", err)
	}

	checkData, err := adtxml.UnmarshalASXData[adtxml.TransportCheckData](data)
	if err != nil {
		return nil, fmt.Errorf("CheckTransport parsing: %w", err)
	}

	result := &TransportCheckResult{
		PgmID:      checkData.PgmID,
		Object:     checkData.Object,
		ObjectName: checkData.ObjectName,
		DevClass:   checkData.DevClass,
		Result:     checkData.Result,
		Recording:  checkData.Recording == "X",
	}
	for _, req := range checkData.Requests {
		result.Requests = append(result.Requests, TransportRequest{
			Number:      req.Header.TrKorr,
			Description: req.Header.Text,
			Status:      req.Header.TrStatus,
		})
	}
	return result, nil
}

func (c *httpClient) CreateTransport(ctx context.Context, category, target, description, devClass string) (string, error) {
	reqData := adtxml.CreateTransportData{
		Category:    strings.ToUpper(category),
		Target:      strings.ToUpper(target),
		Description: description,
		DevClass:    strings.ToUpper(devClass),
	}
	body, err := adtxml.MarshalASXData(reqData)
	if err != nil {
		return "", fmt.Errorf("CreateTransport marshal: %w", err)
	}

	resp, err := c.doMutate(ctx, http.MethodPost,
		"/sap/bc/adt/cts/transports",
		strings.NewReader(string(body)),
		map[string]string{
			// The Content-Type controls the response format: v1 returns plain text,
			// dataname=...CreateCorrectionRequest.v1 returns ASX XML with TRKORR.
			"Content-Type": "application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.CreateCorrectionRequest.v1",
			"Accept":       "application/vnd.sap.as+xml",
		},
	)
	if err != nil {
		return "", fmt.Errorf("CreateTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, _ := io.ReadAll(resp.Body)
	if len(data) > 0 {
		asxData, err := adtxml.UnmarshalASXData[struct {
			TrKorr string `xml:"TRKORR"`
		}](data)
		if err == nil && asxData.TrKorr != "" {
			return asxData.TrKorr, nil
		}
	}

	return "", fmt.Errorf("CreateTransport: transport created but number not returned — check GetTransportRequests to find it")
}

func (c *httpClient) ReleaseTransport(ctx context.Context, transportNumber string) error {
	path := "/sap/bc/adt/cts/transportrequests/" + transportNumber + "/newreleasejobs"
	resp, err := c.doMutate(ctx, http.MethodPost, path, nil,
		map[string]string{"Accept": "application/vnd.sap.adt.transportorganizer.v1+xml"},
	)
	if err != nil {
		return fmt.Errorf("ReleaseTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

func (c *httpClient) GetTransportRequests(ctx context.Context, user, status string) ([]TransportRequest, error) {
	params := url.Values{}
	if user != "" {
		params.Set("user", user)
	}
	if status != "" {
		params.Set("status", status)
	}
	path := "/sap/bc/adt/cts/transportrequests"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/vnd.sap.adt.transportorganizertree.v1+xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportRequests: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, _ := io.ReadAll(resp.Body)
	var root adtxml.TransportRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("GetTransportRequests parsing: %w", err)
	}

	result := make([]TransportRequest, len(root.WorkbenchRequests))
	for i, r := range root.WorkbenchRequests {
		result[i] = TransportRequest{
			Number: r.Number, Owner: r.Owner,
			Description: r.Description, Status: r.Status,
		}
	}
	return result, nil
}

func (c *httpClient) AddToTransport(ctx context.Context, objectURI, transport string) error {
	body, err := xml.Marshal(adtxml.TransportComponent{
		NSCore:    nsADTCore,
		ObjectURI: objectURI,
	})
	if err != nil {
		return fmt.Errorf("marshal transport component: %w", err)
	}

	path := "/sap/bc/adt/cts/transportrequests/" + transport + "/abaptransportcomponents"
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(xml.Header+string(body)),
		map[string]string{"Content-Type": contentTypeXML},
	)
	if err != nil {
		return fmt.Errorf("AddToTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

// TransportObject describes an object recorded in a transport request.
type TransportObject struct {
	PgmID  string `json:"pgmid"`   // R3TR, LIMU
	Type   string `json:"type"`    // PROG, CLAS, DDLS, etc.
	Name   string `json:"name"`    // object name
	WBType string `json:"wb_type"` // e.g. PROG/P, CLAS/OC
}

// GetTransportObjects reads the object list of a transport request
// via GET /sap/bc/adt/cts/transportrequests/{number}.
func (c *httpClient) GetTransportObjects(ctx context.Context, transportNumber string) ([]TransportObject, error) {
	path := "/sap/bc/adt/cts/transportrequests/" + url.PathEscape(transportNumber)
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportObjects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetTransportObjects: reading body: %w", err)
	}

	return parseTransportObjectsXML(data)
}

func parseTransportObjectsXML(data []byte) ([]TransportObject, error) {
	var doc struct {
		Workbench struct {
			Sections []struct {
				Requests []struct {
					Objects []struct {
						PgmID  string `xml:"pgmid,attr"`
						Type   string `xml:"type,attr"`
						Name   string `xml:"name,attr"`
						WBType string `xml:"wbtype,attr"`
					} `xml:"abap_object"`
					Tasks []struct {
						Objects []struct {
							PgmID  string `xml:"pgmid,attr"`
							Type   string `xml:"type,attr"`
							Name   string `xml:"name,attr"`
							WBType string `xml:"wbtype,attr"`
						} `xml:"abap_object"`
					} `xml:"task"`
				} `xml:"request"`
			} `xml:",any"`
		} `xml:"workbench"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing transport objects: %w", err)
	}

	seen := make(map[string]bool)
	var objects []TransportObject

	addObj := func(pgmid, typ, name, wbtype string) {
		key := pgmid + "/" + typ + "/" + name
		if !seen[key] && name != "" {
			seen[key] = true
			objects = append(objects, TransportObject{
				PgmID: pgmid, Type: typ, Name: name, WBType: wbtype,
			})
		}
	}

	for _, section := range doc.Workbench.Sections {
		for _, req := range section.Requests {
			for _, obj := range req.Objects {
				addObj(obj.PgmID, obj.Type, obj.Name, obj.WBType)
			}
			for _, task := range req.Tasks {
				for _, obj := range task.Objects {
					addObj(obj.PgmID, obj.Type, obj.Name, obj.WBType)
				}
			}
		}
	}
	return objects, nil
}
