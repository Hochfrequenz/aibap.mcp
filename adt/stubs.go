package adt

import "context"

func (c *httpClient) ActivateObject(_ context.Context, _ string) (*ActivationResult, error) {
	return nil, nil
}

func (c *httpClient) SearchObjects(_ context.Context, _, _ string, _ int) ([]ObjectInfo, error) {
	return nil, nil
}

func (c *httpClient) WhereUsed(_ context.Context, _ string) ([]ObjectInfo, error) {
	return nil, nil
}

func (c *httpClient) BrowsePackage(_ context.Context, _ string) ([]ObjectInfo, error) {
	return nil, nil
}

func (c *httpClient) GetObjectInfo(_ context.Context, _ string) (*ObjectInfo, error) {
	return nil, nil
}

func (c *httpClient) SyntaxCheck(_ context.Context, _ string) ([]SyntaxMessage, error) {
	return nil, nil
}

func (c *httpClient) RunUnitTests(_ context.Context, _ string, _ int) (*TestResult, error) {
	return nil, nil
}

func (c *httpClient) GetTransportRequests(_ context.Context, _, _ string) ([]TransportRequest, error) {
	return nil, nil
}

func (c *httpClient) AddToTransport(_ context.Context, _, _ string) error {
	return nil
}
