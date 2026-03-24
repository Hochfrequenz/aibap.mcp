// Package sapxml provides types and helpers for SAP ADT XML formats.
//
// This package is designed to be extractable to a separate repository once stable.
// Only add types here that have been verified against a real SAP system.
package sapxml

import (
	"encoding/xml"
	"fmt"
)

// asxEnvelope is the generic wrapper for SAP's asx:abap XML format.
type asxEnvelope[T any] struct {
	XMLName xml.Name     `xml:"abap"`
	Values  asxValues[T] `xml:"values"`
}

type asxValues[T any] struct {
	Data T `xml:"DATA"`
}

// asxEnvelopeMarshal uses explicit namespace attributes for marshalling.
type asxEnvelopeMarshal[T any] struct {
	XMLName xml.Name            `xml:"asx:abap"`
	NS      string              `xml:"xmlns:asx,attr"`
	Version string              `xml:"version,attr"`
	Values  asxValuesMarshal[T] `xml:"asx:values"`
}

type asxValuesMarshal[T any] struct {
	Data T `xml:"DATA"`
}

// UnmarshalASXData extracts the DATA element from an asx:abap XML envelope
// and unmarshals its content into T.
func UnmarshalASXData[T any](data []byte) (*T, error) {
	var env asxEnvelope[T]
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal asx:abap: %w", err)
	}
	return &env.Values.Data, nil
}

// MarshalASXData wraps the given struct in an asx:abap XML envelope.
func MarshalASXData[T any](source T) ([]byte, error) {
	env := asxEnvelopeMarshal[T]{
		NS:      "http://www.sap.com/abapxml",
		Version: "1.0",
		Values:  asxValuesMarshal[T]{Data: source},
	}
	out, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal asx:abap: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}
