package sdk

import "net/http"

type HealthService struct {
	http *HTTPClient
}

func (s *HealthService) Ping() (string, error) {
	return s.http.DoText(http.MethodGet, "/health")
}
