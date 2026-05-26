package sdk

import "net/http"

type CollectionScope struct {
	http *HTTPClient
	name string
}

func (s *CollectionScope) Name() string {
	return s.name
}

func (s *CollectionScope) Documents() *DocumentsService {
	return &DocumentsService{http: s.http, collection: s.name}
}

func (s *CollectionScope) Indexes() *IndexesService {
	return &IndexesService{http: s.http, collection: s.name}
}

func (s *CollectionScope) Stats() (*CollectionStats, error) {
	var st CollectionStats
	err := s.http.DoJSON(http.MethodGet, "/api/collections/"+Enc(s.name)+"/stats", nil, &st)
	return &st, err
}
