package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	ctx       context.Context
	serverURL string
	client    *http.Client
}

type clientConfig struct {
	BaseURL string `json:"baseURL"`
}

func configFilePath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(d, "evelent-db")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "client.json"), nil
}

func loadServerURL() string {
	p, err := configFilePath()
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	var c clientConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return "http://127.0.0.1:8080"
	}
	if c.BaseURL == "" {
		return "http://127.0.0.1:8080"
	}
	u, err := normalizeServerURL(c.BaseURL)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	return u
}

func saveServerURL(base string) error {
	p, err := configFilePath()
	if err != nil {
		return err
	}
	c := clientConfig{BaseURL: base}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func normalizeServerURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "http://127.0.0.1:8080", nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("host required")
	}
	out := u.Scheme + "://" + u.Host
	return strings.TrimSuffix(out, "/"), nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          2048,
			MaxIdleConnsPerHost:   2048,
			MaxConnsPerHost:       0,
			IdleConnTimeout:       120 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

func NewApp() *App {
	u := loadServerURL()
	return &App{
		serverURL: u,
		client:    newHTTPClient(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) base() string {
	return strings.TrimSuffix(a.serverURL, "/")
}

func (a *App) GetServerURL() string {
	return a.serverURL
}

func (a *App) SetServerURL(raw string) error {
	u, err := normalizeServerURL(raw)
	if err != nil {
		return err
	}
	if err := saveServerURL(u); err != nil {
		return err
	}
	a.serverURL = u
	a.client = newHTTPClient()
	return nil
}

func (a *App) PingServer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base()+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func (a *App) ctxOrBg() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) getJSON(url string, out interface{}) error {
	req, err := http.NewRequestWithContext(a.ctxOrBg(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", body)
	}
	return json.Unmarshal(body, out)
}

func (a *App) ListCollections() ([]string, error) {
	var collections []string
	err := a.getJSON(a.base()+"/api/collections", &collections)
	if collections == nil {
		collections = []string{}
	}
	return collections, err
}

func (a *App) CreateCollection(name string) error {
	payload := map[string]string{"name": name}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := a.client.Post(a.base()+"/api/collections", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) DropCollection(name string) error {
	req, err := http.NewRequest(http.MethodDelete, a.base()+"/api/collections/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) InsertDocument(collection string, doc map[string]interface{}) (string, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	resp, err := a.client.Post(a.base()+"/api/collections/"+url.PathEscape(collection)+"/docs", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", body)
	}
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result["_id"], nil
}

func (a *App) GetDocument(collection, id string) (map[string]interface{}, error) {
	var doc map[string]interface{}
	err := a.getJSON(a.base()+"/api/collections/"+url.PathEscape(collection)+"/docs/"+url.PathEscape(id), &doc)
	return doc, err
}

func (a *App) UpdateDocument(collection, id string, update map[string]interface{}) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, a.base()+"/api/collections/"+url.PathEscape(collection)+"/docs/"+url.PathEscape(id), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) DeleteDocument(collection, id string) error {
	req, err := http.NewRequest(http.MethodDelete, a.base()+"/api/collections/"+url.PathEscape(collection)+"/docs/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) FindDocuments(collection string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	return a.FindDocumentsQuery(collection, filter)
}

func (a *App) FindDocumentsQuery(collection string, query map[string]interface{}) ([]map[string]interface{}, error) {
	data, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Post(a.base()+"/api/collections/"+url.PathEscape(collection)+"/find", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", body)
	}
	var docs []map[string]interface{}
	if err := json.Unmarshal(body, &docs); err == nil {
		if docs == nil {
			docs = []map[string]interface{}{}
		}
		return docs, nil
	}
	var wrap struct {
		Documents []map[string]interface{} `json:"documents"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, err
	}
	if wrap.Documents == nil {
		wrap.Documents = []map[string]interface{}{}
	}
	return wrap.Documents, nil
}

func (a *App) CreateIndex(collection, field string) error {
	payload := map[string]string{"field": field}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := a.client.Post(a.base()+"/api/collections/"+url.PathEscape(collection)+"/indexes", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) ListIndexes(collection string) ([]string, error) {
	var indexes []string
	err := a.getJSON(a.base()+"/api/collections/"+url.PathEscape(collection)+"/indexes", &indexes)
	if indexes == nil {
		indexes = []string{}
	}
	return indexes, err
}

func (a *App) DropIndex(collection, field string) error {
	req, err := http.NewRequest(http.MethodDelete, a.base()+"/api/collections/"+url.PathEscape(collection)+"/indexes/"+url.PathEscape(field), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%s", body)
	}
	return nil
}

func (a *App) CollectionStats(collection string) (map[string]interface{}, error) {
	var stats map[string]interface{}
	err := a.getJSON(a.base()+"/api/collections/"+url.PathEscape(collection)+"/stats", &stats)
	return stats, err
}

// ---------------------------------------------------------------------------
// In-memory KV store bindings
// ---------------------------------------------------------------------------

// KVKeys lists live keys in the in-memory store, optionally filtered by prefix.
func (a *App) KVKeys(prefix string) ([]string, error) {
	u := a.base() + "/api/kv/keys"
	if strings.TrimSpace(prefix) != "" {
		u += "?prefix=" + url.QueryEscape(prefix)
	}
	var out struct {
		Keys []string `json:"keys"`
	}
	if err := a.getJSON(u, &out); err != nil {
		return []string{}, err
	}
	if out.Keys == nil {
		out.Keys = []string{}
	}
	return out.Keys, nil
}

// KVGet returns the value for key (empty string when absent).
func (a *App) KVGet(key string) (string, error) {
	req, err := http.NewRequestWithContext(a.ctxOrBg(), http.MethodGet, a.base()+"/api/kv/"+url.PathEscape(key), nil)
	if err != nil {
		return "", err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", body)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	return out.Value, nil
}

// KVSet stores value under key. ttlSeconds <= 0 means no expiry.
func (a *App) KVSet(key, value string, ttlSeconds float64) error {
	payload := map[string]interface{}{"value": value, "ttlSeconds": ttlSeconds}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, a.base()+"/api/kv/"+url.PathEscape(key), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", body)
	}
	return nil
}

// KVDelete removes a key.
func (a *App) KVDelete(key string) error {
	req, err := http.NewRequest(http.MethodDelete, a.base()+"/api/kv/"+url.PathEscape(key), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%s", body)
	}
	return nil
}

// KVIncr atomically adds delta to the integer at key and returns the new value.
func (a *App) KVIncr(key string, delta int64) (int64, error) {
	payload := map[string]interface{}{"delta": delta}
	data, _ := json.Marshal(payload)
	resp, err := a.client.Post(a.base()+"/api/kv/"+url.PathEscape(key)+"/incr", "application/json", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("%s", body)
	}
	var out struct {
		Value int64 `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Value, nil
}

// KVStats returns the in-memory store counters.
func (a *App) KVStats() (map[string]interface{}, error) {
	var stats map[string]interface{}
	err := a.getJSON(a.base()+"/api/kv/stats", &stats)
	return stats, err
}

// KVFlush removes every key from the in-memory store.
func (a *App) KVFlush() error {
	resp, err := a.client.Post(a.base()+"/api/kv/flush", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", body)
	}
	return nil
}
