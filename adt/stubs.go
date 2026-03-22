package adt

import "context"

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
