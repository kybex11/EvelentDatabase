package sdk

type Client struct {
	HTTP        *HTTPClient
	Collections *CollectionsService
	Health      *HealthService
}

func New(baseURL string) *Client {
	h := NewHTTPClient(baseURL)
	return &Client{
		HTTP:        h,
		Collections: &CollectionsService{http: h},
		Health:      &HealthService{http: h},
	}
}

func (c *Client) Collection(name string) *CollectionScope {
	return &CollectionScope{http: c.HTTP, name: name}
}
