package sdk

import (
	"net/http"
	"net/url"
)

// KVService is a client for the embedded in-memory key/value store. It mirrors
// a Redis-like surface (set/get/del, TTL, atomic counters, batch ops) over the
// database's HTTP API.
type KVService struct {
	http *HTTPClient
}

// KVStats is a snapshot of the in-memory store counters.
type KVStats struct {
	Items     int     `json:"items"`
	Bytes     int64   `json:"bytes"`
	MaxItems  int     `json:"maxItems"`
	MaxBytes  int64   `json:"maxBytes"`
	Hits      uint64  `json:"hits"`
	Misses    uint64  `json:"misses"`
	Evictions uint64  `json:"evictions"`
	Expired   uint64  `json:"expired"`
	HitRatio  float64 `json:"hitRatio"`
}

// KVItem is a single key/value pair used by batch operations.
type KVItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func kvPath(key string) string {
	return "/api/kv/" + url.PathEscape(key)
}

// Set stores value under key. ttlSeconds <= 0 stores it without expiry.
func (s *KVService) Set(key, value string, ttlSeconds float64) error {
	return s.http.DoJSON(http.MethodPut, kvPath(key), map[string]interface{}{
		"value":      value,
		"ttlSeconds": ttlSeconds,
	}, nil)
}

// SetNX stores value only if key does not already exist. It returns false (and
// no error) when the key was already present.
func (s *KVService) SetNX(key, value string, ttlSeconds float64) (bool, error) {
	err := s.http.DoJSON(http.MethodPut, kvPath(key), map[string]interface{}{
		"value":      value,
		"ttlSeconds": ttlSeconds,
		"nx":         true,
	}, nil)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusConflict {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Get returns the value for key. found is false when the key is absent.
func (s *KVService) Get(key string) (value string, found bool, err error) {
	var out struct {
		Value string `json:"value"`
	}
	err = s.http.DoJSON(http.MethodGet, kvPath(key), nil, &out)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return out.Value, true, nil
}

// Delete removes key. found is false when the key did not exist.
func (s *KVService) Delete(key string) (found bool, err error) {
	err = s.http.DoJSON(http.MethodDelete, kvPath(key), nil, nil)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Incr atomically adds delta to the integer stored at key and returns the new
// value, creating the key at 0 when absent.
func (s *KVService) Incr(key string, delta int64) (int64, error) {
	var out struct {
		Value int64 `json:"value"`
	}
	err := s.http.DoJSON(http.MethodPost, kvPath(key)+"/incr", map[string]interface{}{
		"delta": delta,
	}, &out)
	return out.Value, err
}

// Expire sets or clears a key's TTL (ttlSeconds <= 0 clears it).
func (s *KVService) Expire(key string, ttlSeconds float64) error {
	return s.http.DoJSON(http.MethodPost, kvPath(key)+"/expire", map[string]interface{}{
		"ttlSeconds": ttlSeconds,
	}, nil)
}

// TTL returns the remaining lifetime in seconds. persists is true when the key
// has no expiry.
func (s *KVService) TTL(key string) (ttlSeconds float64, persists bool, err error) {
	var out struct {
		TTLSeconds float64 `json:"ttlSeconds"`
		Persists   bool    `json:"persists"`
	}
	err = s.http.DoJSON(http.MethodGet, kvPath(key)+"/ttl", nil, &out)
	return out.TTLSeconds, out.Persists, err
}

// Keys returns all live keys, optionally filtered by prefix.
func (s *KVService) Keys(prefix string) ([]string, error) {
	p := "/api/kv/keys"
	if prefix != "" {
		p += "?prefix=" + url.QueryEscape(prefix)
	}
	var out struct {
		Keys []string `json:"keys"`
	}
	err := s.http.DoJSON(http.MethodGet, p, nil, &out)
	return out.Keys, err
}

// Stats returns the in-memory store counters.
func (s *KVService) Stats() (*KVStats, error) {
	var st KVStats
	err := s.http.DoJSON(http.MethodGet, "/api/kv/stats", nil, &st)
	return &st, err
}

// Flush removes every key.
func (s *KVService) Flush() error {
	return s.http.DoJSON(http.MethodPost, "/api/kv/flush", nil, nil)
}

// MSet writes many pairs in one request, applying the same ttl to all of them.
func (s *KVService) MSet(items []KVItem, ttlSeconds float64) (int, error) {
	var out struct {
		Written int `json:"written"`
	}
	err := s.http.DoJSON(http.MethodPost, "/api/kv/mset", map[string]interface{}{
		"items":      items,
		"ttlSeconds": ttlSeconds,
	}, &out)
	return out.Written, err
}

// MGet reads many keys in one request. Missing keys are reported separately.
func (s *KVService) MGet(keys []string) (items []KVItem, missing []string, err error) {
	var out struct {
		Items   []KVItem `json:"items"`
		Missing []string `json:"missing"`
	}
	err = s.http.DoJSON(http.MethodPost, "/api/kv/mget", map[string]interface{}{
		"keys": keys,
	}, &out)
	return out.Items, out.Missing, err
}

// MDelete removes many keys in one request and returns how many existed.
func (s *KVService) MDelete(keys []string) (int, error) {
	var out struct {
		Deleted int `json:"deleted"`
	}
	err := s.http.DoJSON(http.MethodPost, "/api/kv/mdel", map[string]interface{}{
		"keys": keys,
	}, &out)
	return out.Deleted, err
}
