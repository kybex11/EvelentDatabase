package sdk

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type DocumentsService struct {
	http       *HTTPClient
	collection string
}

func (d *DocumentsService) base() string {
	return "/api/collections/" + Enc(d.collection)
}

func (d *DocumentsService) InsertOne(doc map[string]interface{}) (*InsertOneResult, error) {
	var out InsertOneResult
	err := d.http.DoJSON(http.MethodPost, d.base()+"/docs", doc, &out)
	return &out, err
}

func (d *DocumentsService) InsertMany(documents []map[string]interface{}) (*InsertManyResult, error) {
	var out InsertManyResult
	err := d.http.DoJSON(http.MethodPost, d.base()+"/docs/batch", map[string]interface{}{"documents": documents}, &out)
	return &out, err
}

func (d *DocumentsService) GetByID(id string) (map[string]interface{}, error) {
	var out map[string]interface{}
	err := d.http.DoJSON(http.MethodGet, d.base()+"/docs/"+Enc(id), nil, &out)
	return out, err
}

func (d *DocumentsService) ReplaceByID(id string, doc map[string]interface{}) error {
	return d.http.DoJSON(http.MethodPut, d.base()+"/docs/"+Enc(id), doc, nil)
}

func (d *DocumentsService) DeleteByID(id string) error {
	req, err := http.NewRequest(http.MethodDelete, d.http.URL(d.base()+"/docs/"+Enc(id)), nil)
	if err != nil {
		return err
	}
	d.http.applyAuth(req)
	resp, err := d.http.HTTPClient.Do(req)
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

func (d *DocumentsService) Find(query FindQuery) ([]map[string]interface{}, error) {
	res, err := d.FindPage(query)
	if err != nil {
		return nil, err
	}
	return res.Documents, nil
}

// FindPage runs a find and returns documents plus nextCursor for keyset pagination
// (pass cursor or after in the next query).
func (d *DocumentsService) FindPage(query FindQuery) (*FindResult, error) {
	if query == nil {
		query = FindQuery{}
	}
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, d.http.URL(d.base()+"/find"), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	d.http.applyAuth(req)
	resp, err := d.http.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, StatusText: resp.Status, Body: raw}
	}
	var wrap FindResult
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Documents != nil {
		return &wrap, nil
	}
	var docs []map[string]interface{}
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []map[string]interface{}{}
	}
	return &FindResult{Documents: docs}, nil
}
