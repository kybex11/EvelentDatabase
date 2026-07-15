package sdk

type Client struct {
	HTTP        *HTTPClient
	Collections *CollectionsService
	Health      *HealthService
	KV          *KVService
	PubSub      *PubSubService
}

type Option func(*HTTPClient)

// WithAPIKey attaches X-API-Key to every HTTP request (required when the server
// is started with -api-key / DB_API_KEY).
func WithAPIKey(key string) Option {
	return func(h *HTTPClient) {
		h.APIKey = key
	}
}

func New(baseURL string, opts ...Option) *Client {
	h := NewHTTPClient(baseURL)
	for _, o := range opts {
		o(h)
	}
	return &Client{
		HTTP:        h,
		Collections: &CollectionsService{http: h},
		Health:      &HealthService{http: h},
		KV:          &KVService{http: h},
		PubSub:      &PubSubService{http: h},
	}
}

func (c *Client) Collection(name string) *CollectionScope {
	return &CollectionScope{http: c.HTTP, name: name}
}
