package adt

import "context"

func (c *httpClient) GetTransportRequests(_ context.Context, _, _ string) ([]TransportRequest, error) {
	return nil, nil
}

func (c *httpClient) AddToTransport(_ context.Context, _, _ string) error {
	return nil
}
