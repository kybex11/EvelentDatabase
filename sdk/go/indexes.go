package sdk

import (
	"io"
	"net/http"
)

type IndexesService struct {
	http       *HTTPClient
	collection string
}

func (s *IndexesService) base() string {
	return "/api/collections/" + Enc(s.collection)
}

func (s *IndexesService) List() ([]string, error) {
	var out []string
	err := s.http.DoJSON(http.MethodGet, s.base()+"/indexes", nil, &out)
	if out == nil {
		out = []string{}
	}
	return out, err
}

func (s *IndexesService) Create(field string) error {
	return s.http.DoJSON(http.MethodPost, s.base()+"/indexes", map[string]string{"field": field}, nil)
}

func (s *IndexesService) Drop(field string) error {
	req, err := http.NewRequest(http.MethodDelete, s.http.URL(s.base()+"/indexes/"+Enc(field)), nil)
	if err != nil {
		return err
	}
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
