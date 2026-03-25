package adtmodel

import (
	"encoding/xml"
	"testing"
)

func TestUnmarshalDataPreviewResult_TwoColumnsTwoRows(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?>
<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">
  <dataPreview:totalRows>67</dataPreview:totalRows>
  <dataPreview:isHanaAnalyticalView>false</dataPreview:isHanaAnalyticalView>
  <dataPreview:executedQueryString>SELECT * FROM T001</dataPreview:executedQueryString>
  <dataPreview:queryExecutionTime>0.3600000</dataPreview:queryExecutionTime>
  <dataPreview:columns>
    <dataPreview:metadata dataPreview:name="BUKRS" dataPreview:type="C"
      dataPreview:description="Company Code" dataPreview:keyAttribute="true"
      dataPreview:colType="" dataPreview:isKeyFigure="false"/>
    <dataPreview:dataSet>
      <dataPreview:data>0001</dataPreview:data>
      <dataPreview:data>0003</dataPreview:data>
    </dataPreview:dataSet>
  </dataPreview:columns>
  <dataPreview:columns>
    <dataPreview:metadata dataPreview:name="BUTXT" dataPreview:type="C"
      dataPreview:description="Company Name" dataPreview:keyAttribute="false"
      dataPreview:colType="" dataPreview:isKeyFigure="false"/>
    <dataPreview:dataSet>
      <dataPreview:data>SAP SE</dataPreview:data>
      <dataPreview:data>SAP US</dataPreview:data>
    </dataPreview:dataSet>
  </dataPreview:columns>
</dataPreview:tableData>`

	var result DataPreviewResult
	if err := xml.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != "67" {
		t.Errorf("TotalRows: got %q, want %q", result.TotalRows, "67")
	}
	if result.IsHanaAnalyticalView != "false" {
		t.Errorf("IsHanaAnalyticalView: got %q, want %q", result.IsHanaAnalyticalView, "false")
	}
	if result.ExecutedQueryString != "SELECT * FROM T001" {
		t.Errorf("ExecutedQueryString: got %q", result.ExecutedQueryString)
	}
	if result.QueryExecutionTime != "0.3600000" {
		t.Errorf("QueryExecutionTime: got %q", result.QueryExecutionTime)
	}

	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result.Columns))
	}

	// First column: BUKRS
	col0 := result.Columns[0]
	if col0.Metadata.Name != "BUKRS" {
		t.Errorf("col[0].Metadata.Name: got %q", col0.Metadata.Name)
	}
	if col0.Metadata.Type != "C" {
		t.Errorf("col[0].Metadata.Type: got %q", col0.Metadata.Type)
	}
	if col0.Metadata.Description != "Company Code" {
		t.Errorf("col[0].Metadata.Description: got %q", col0.Metadata.Description)
	}
	if col0.Metadata.KeyAttribute != "true" {
		t.Errorf("col[0].Metadata.KeyAttribute: got %q", col0.Metadata.KeyAttribute)
	}
	if len(col0.DataSet.Data) != 2 {
		t.Fatalf("col[0].DataSet: expected 2 entries, got %d", len(col0.DataSet.Data))
	}
	if col0.DataSet.Data[0] != "0001" {
		t.Errorf("col[0].DataSet.Data[0]: got %q", col0.DataSet.Data[0])
	}
	if col0.DataSet.Data[1] != "0003" {
		t.Errorf("col[0].DataSet.Data[1]: got %q", col0.DataSet.Data[1])
	}

	// Second column: BUTXT
	col1 := result.Columns[1]
	if col1.Metadata.Name != "BUTXT" {
		t.Errorf("col[1].Metadata.Name: got %q", col1.Metadata.Name)
	}
	if col1.Metadata.Description != "Company Name" {
		t.Errorf("col[1].Metadata.Description: got %q", col1.Metadata.Description)
	}
	if col1.Metadata.KeyAttribute != "false" {
		t.Errorf("col[1].Metadata.KeyAttribute: got %q", col1.Metadata.KeyAttribute)
	}
	if len(col1.DataSet.Data) != 2 {
		t.Fatalf("col[1].DataSet: expected 2 entries, got %d", len(col1.DataSet.Data))
	}
	if col1.DataSet.Data[0] != "SAP SE" {
		t.Errorf("col[1].DataSet.Data[0]: got %q", col1.DataSet.Data[0])
	}
	if col1.DataSet.Data[1] != "SAP US" {
		t.Errorf("col[1].DataSet.Data[1]: got %q", col1.DataSet.Data[1])
	}
}

func TestUnmarshalDataPreviewResult_EmptyResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?>
<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">
  <dataPreview:totalRows>0</dataPreview:totalRows>
  <dataPreview:isHanaAnalyticalView>false</dataPreview:isHanaAnalyticalView>
  <dataPreview:executedQueryString>SELECT * FROM ZEMPTY</dataPreview:executedQueryString>
  <dataPreview:queryExecutionTime>0.0010000</dataPreview:queryExecutionTime>
</dataPreview:tableData>`

	var result DataPreviewResult
	if err := xml.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != "0" {
		t.Errorf("TotalRows: got %q, want %q", result.TotalRows, "0")
	}
	if len(result.Columns) != 0 {
		t.Errorf("expected 0 columns, got %d", len(result.Columns))
	}
}

func TestUnmarshalDataPreviewResult_ColumnWithEmptyDataSet(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8"?>
<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">
  <dataPreview:totalRows>0</dataPreview:totalRows>
  <dataPreview:isHanaAnalyticalView>false</dataPreview:isHanaAnalyticalView>
  <dataPreview:executedQueryString>SELECT * FROM T001 WHERE 1=0</dataPreview:executedQueryString>
  <dataPreview:queryExecutionTime>0.0020000</dataPreview:queryExecutionTime>
  <dataPreview:columns>
    <dataPreview:metadata dataPreview:name="BUKRS" dataPreview:type="C"
      dataPreview:description="Company Code" dataPreview:keyAttribute="true"
      dataPreview:colType="" dataPreview:isKeyFigure="false"/>
    <dataPreview:dataSet/>
  </dataPreview:columns>
</dataPreview:tableData>`

	var result DataPreviewResult
	if err := xml.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(result.Columns))
	}

	col := result.Columns[0]
	if col.Metadata.Name != "BUKRS" {
		t.Errorf("Metadata.Name: got %q", col.Metadata.Name)
	}
	if col.Metadata.Type != "C" {
		t.Errorf("Metadata.Type: got %q", col.Metadata.Type)
	}
	if col.Metadata.Description != "Company Code" {
		t.Errorf("Metadata.Description: got %q", col.Metadata.Description)
	}
	if col.Metadata.KeyAttribute != "true" {
		t.Errorf("Metadata.KeyAttribute: got %q", col.Metadata.KeyAttribute)
	}
	if len(col.DataSet.Data) != 0 {
		t.Errorf("expected 0 data entries, got %d", len(col.DataSet.Data))
	}
}
