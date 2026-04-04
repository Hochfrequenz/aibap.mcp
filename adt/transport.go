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
		Text:        description,
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

func (c *httpClient) CreateTransportTask(ctx context.Context, parentTransport, owner, description string) (string, error) {
	var descBuf strings.Builder
	_ = xml.EscapeText(&descBuf, []byte(description))
	if owner == "" {
		owner = c.cfg.User
	}
	owner = strings.ToUpper(owner)
	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm"` +
		` tm:useraction="newtask" tm:targetuser="` + owner + `">` +
		`<tm:task tm:owner="` + owner + `" tm:desc="` + descBuf.String() + `"/>` +
		`</tm:root>`

	path := "/sap/bc/adt/cts/transportrequests/" + parentTransport + "/tasks"
	resp, err := c.doMutate(ctx, http.MethodPost, path,
		strings.NewReader(body),
		map[string]string{
			"Content-Type": "application/vnd.sap.adt.transportorganizer.v1+xml",
			"Accept":       "application/vnd.sap.adt.transportorganizer.v1+xml",
		},
	)
	if err != nil {
		return "", fmt.Errorf("CreateTransportTask: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return "", err
	}

	data, _ := io.ReadAll(resp.Body)
	if len(data) > 0 {
		// The response has the task number as tm:number attribute on tm:root.
		var taskRoot struct {
			Number string `xml:"number,attr"`
		}
		if err := xml.Unmarshal(data, &taskRoot); err == nil && taskRoot.Number != "" {
			return taskRoot.Number, nil
		}
	}

	return "", fmt.Errorf("CreateTransportTask: task created but number not returned")
}

func (c *httpClient) DeleteTransport(ctx context.Context, transportNumber string) error {
	path := "/sap/bc/adt/cts/transportrequests/" + transportNumber
	resp, err := c.doMutate(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return fmt.Errorf("DeleteTransport: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return checkResponse(resp)
}

// ReleaseTransport releases a transport request or task.
// If the request has unreleased tasks, it returns an error listing them.
func (c *httpClient) ReleaseTransport(ctx context.Context, transportNumber string) error {
	return c.releaseTransport(ctx, transportNumber, false)
}

// ReleaseTransportWithTasks releases a transport request including all its tasks.
// Each task is released first, then the request itself.
func (c *httpClient) ReleaseTransportWithTasks(ctx context.Context, transportNumber string) error {
	return c.releaseTransport(ctx, transportNumber, true)
}

func (c *httpClient) releaseTransport(ctx context.Context, transportNumber string, releaseTasks bool) error {
	err := c.releaseTransportDirect(ctx, transportNumber)
	if err == nil {
		return nil
	}

	// Check for unreleased tasks.
	tasks, taskErr := c.GetTransportTasks(ctx, transportNumber)
	if taskErr != nil || len(tasks) == 0 {
		return err
	}

	if !releaseTasks {
		return fmt.Errorf("transport %s has %d unreleased task(s): %s — release them first or use ReleaseTransportWithTasks",
			transportNumber, len(tasks), strings.Join(tasks, ", "))
	}

	for _, task := range tasks {
		if taskReleaseErr := c.releaseTransportDirect(ctx, task); taskReleaseErr != nil {
			return fmt.Errorf("ReleaseTransport: releasing task %s failed: %w", task, taskReleaseErr)
		}
	}
	return c.releaseTransportDirect(ctx, transportNumber)
}

// releaseTransportDirect releases a single transport or task.
// Tasks use /releasejobs (works on ECC and S4).
// Requests use /newreleasejobs (S4 only — ECC returns 400, see #224).
// Falls back to /releasejobs if /newreleasejobs is not available.
func (c *httpClient) releaseTransportDirect(ctx context.Context, transportNumber string) error {
	headers := map[string]string{
		"Accept":                "application/vnd.sap.adt.transportorganizer.v1+xml",
		"X-sap-adt-sessiontype": "stateful",
	}

	// Try /newreleasejobs first (works for requests on S4).
	path := "/sap/bc/adt/cts/transportrequests/" + transportNumber + "/newreleasejobs"
	resp, err := c.doMutate(ctx, http.MethodPost, path, nil, headers)
	if err != nil {
		return fmt.Errorf("ReleaseTransport %s: %w", transportNumber, err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode < 400 {
		return checkReleaseResponse(transportNumber, data)
	}

	// Fallback to /releasejobs (works for tasks on ECC and S4).
	path = "/sap/bc/adt/cts/transportrequests/" + transportNumber + "/releasejobs"
	resp, err = c.doMutate(ctx, http.MethodPost, path, nil, headers)
	if err != nil {
		return fmt.Errorf("ReleaseTransport %s: %w", transportNumber, err)
	}
	data, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return err
	}
	return checkReleaseResponse(transportNumber, data)
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

// checkReleaseResponse parses the release job response and returns an error
// if the release failed. SAP returns HTTP 200 even on failure — the actual
// status is in chkrun:status attribute ("released" = OK, "abortrelapifail" = error).
func checkReleaseResponse(transportNumber string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var root struct {
		Reports struct {
			Report struct {
				Status     string `xml:"status,attr"`
				StatusText string `xml:"statusText,attr"`
				Messages   struct {
					Items []struct {
						Type      string `xml:"type,attr"`
						ShortText string `xml:"shortText,attr"`
					} `xml:"checkMessage"`
				} `xml:"checkMessageList"`
			} `xml:"checkReport"`
		} `xml:"releasereports"`
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil // can't parse — assume OK
	}
	if root.Reports.Report.Status == "" || root.Reports.Report.Status == "released" {
		return nil
	}
	// Collect error messages
	msg := root.Reports.Report.StatusText
	for _, m := range root.Reports.Report.Messages.Items {
		if m.Type == "E" {
			msg += ": " + m.ShortText
		}
	}
	return fmt.Errorf("ReleaseTransport %s failed: %s", transportNumber, msg)
}

// TransportObject describes an object recorded in a transport request.
type TransportObject struct {
	PgmID  string `json:"pgmid"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	WBType string `json:"wb_type"`
}

// GetTransportObjects reads the object list of a transport request, deduplicated across request and tasks.
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

// GetTransportTasks returns the task numbers belonging to a transport request.
func (c *httpClient) GetTransportTasks(ctx context.Context, transportNumber string) ([]string, error) {
	path := "/sap/bc/adt/cts/transportrequests/" + url.PathEscape(transportNumber)
	resp, err := c.doRead(ctx, path, map[string]string{"Accept": "application/xml"})
	if err != nil {
		return nil, fmt.Errorf("GetTransportTasks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetTransportTasks: reading body: %w", err)
	}
	return parseTransportTaskNumbers(data, transportNumber)
}

func parseTransportTaskNumbers(data []byte, transportNumber string) ([]string, error) {
	var doc struct {
		Workbench struct {
			Sections []struct {
				Requests []struct {
					Number string `xml:"number,attr"`
					Tasks  []struct {
						Number string `xml:"number,attr"`
					} `xml:"task"`
				} `xml:"request"`
			} `xml:",any"`
		} `xml:"workbench"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing transport tasks: %w", err)
	}
	var tasks []string
	for _, section := range doc.Workbench.Sections {
		for _, req := range section.Requests {
			if transportNumber != "" && req.Number != transportNumber {
				continue
			}
			for _, task := range req.Tasks {
				if task.Number != "" {
					tasks = append(tasks, task.Number)
				}
			}
		}
	}
	return tasks, nil
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
			objects = append(objects, TransportObject{PgmID: pgmid, Type: typ, Name: name, WBType: wbtype})
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
