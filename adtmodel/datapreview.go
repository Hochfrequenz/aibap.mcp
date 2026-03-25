package adtmodel

import "encoding/xml"

// DataPreviewResult is the response from POST /sap/bc/adt/datapreview/freestyle.
// Content-Type: application/xml
// The data is column-oriented: each Column has metadata and a DataSet with
// one entry per row.
// Verified against S/4 HANA (srvhfuhana.sap.msp.local:44300) on 2026-03-25.
type DataPreviewResult struct {
	XMLName              xml.Name            `xml:"tableData"`
	TotalRows            string              `xml:"totalRows"`
	IsHanaAnalyticalView string              `xml:"isHanaAnalyticalView"`
	ExecutedQueryString  string              `xml:"executedQueryString"`
	QueryExecutionTime   string              `xml:"queryExecutionTime"`
	Columns              []DataPreviewColumn `xml:"columns"`
}

// DataPreviewColumn is a single column in a data preview result.
type DataPreviewColumn struct {
	Metadata DataPreviewMetadata `xml:"metadata"`
	DataSet  DataPreviewDataSet  `xml:"dataSet"`
}

// DataPreviewMetadata describes a column (name, ABAP type, description, key flag).
type DataPreviewMetadata struct {
	Name         string `xml:"name,attr"`
	Type         string `xml:"type,attr"`
	Description  string `xml:"description,attr"`
	KeyAttribute string `xml:"keyAttribute,attr"`
	ColType      string `xml:"colType,attr"`
	IsKeyFigure  string `xml:"isKeyFigure,attr"`
}

// DataPreviewDataSet contains the row values for a single column.
type DataPreviewDataSet struct {
	Data []string `xml:"data"`
}
