package adtmodel

import "encoding/xml"

// ObjectReferences is the XML response from search/quickSearch.
type ObjectReferences struct {
	XMLName    xml.Name          `xml:"objectReferences"`
	References []ObjectReference `xml:"objectReference"`
}

// ObjectReference is a single object reference in a search result.
type ObjectReference struct {
	URI         string `xml:"uri,attr"`
	Type        string `xml:"type,attr"`
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	PackageName string `xml:"packageName,attr"`
}

// UsageReferenceResult is the XML response from where-used queries.
type UsageReferenceResult struct {
	XMLName    xml.Name           `xml:"usageReferenceResult"`
	References []ReferencedObject `xml:"referencedObjects>referencedObject"`
}

// ReferencedObject is a single referenced object in a where-used result.
type ReferencedObject struct {
	URI       string    `xml:"uri,attr"`
	ADTObject ADTObject `xml:"adtObject"`
}

// ADTObject describes an ADT object within a usage reference.
type ADTObject struct {
	Name        string `xml:"name,attr"`
	Type        string `xml:"type,attr"`
	Description string `xml:"description,attr"`
	PackageRef  struct {
		Name string `xml:"name,attr"`
	} `xml:"packageRef"`
}
