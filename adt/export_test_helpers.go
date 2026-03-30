package adt

// ParseTransportObjectsXMLForTest exposes parseTransportObjectsXML for unit tests.
func ParseTransportObjectsXMLForTest(data []byte) ([]TransportObject, error) {
	return parseTransportObjectsXML(data)
}
