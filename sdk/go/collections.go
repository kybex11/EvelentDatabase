package sdk

import (
	"io"
	"net/http"
)

type CollectionsService struct {
	http *HTTPClient
}

func (s *CollectionsService) List() ([]string, error) {
	var out []string
	err := s.http.DoJSON(http.MethodGet, "/api/collections", nil, &out)
	return out, err
}

func (s *CollectionsService) Create(name string) error {
	return s.http.DoJSON(http.MethodPost, "/api/collections", map[string]string{"name": name}, nil)
}

func (s *CollectionsService) Drop(name string) error {
	req, err := http.NewRequest(http.MethodDelete, s.http.URL("/api/collections/"+Enc(name)), nil)
	if err != nil {
		return err
	}
	s.http.applyAuth(req)
	resp, err := s.http.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, StatusText: resp.Status, Body: raw}
	}
	return nil
}
