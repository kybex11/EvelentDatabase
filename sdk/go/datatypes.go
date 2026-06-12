package sdk

import (
	"net/http"
	"net/url"
	"strconv"
)

// This file adds Redis-style hash, list and set commands to KVService, all
// backed by the embedded in-memory store.

func hashPath(key string) string { return "/api/hash/" + url.PathEscape(key) }
func listPath(key string) string { return "/api/list/" + url.PathEscape(key) }
func setPath(key string) string  { return "/api/set/" + url.PathEscape(key) }

// ---- Hash ----

// HSet sets field to value in the hash at key. Returns newly-created field count.
func (s *KVService) HSet(key, field, value string) (int, error) {
	var out struct {
		Added int `json:"added"`
	}
	err := s.http.DoJSON(http.MethodPut, hashPath(key)+"/"+url.PathEscape(field),
		map[string]interface{}{"value": value}, &out)
	return out.Added, err
}

// HSetMany sets multiple fields at once.
func (s *KVService) HSetMany(key string, fields map[string]string) (int, error) {
	var out struct {
		Added int `json:"added"`
	}
	err := s.http.DoJSON(http.MethodPut, hashPath(key),
		map[string]interface{}{"fields": fields}, &out)
	return out.Added, err
}

// HGet returns the value of field in the hash at key.
func (s *KVService) HGet(key, field string) (string, bool, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := s.http.DoJSON(http.MethodGet, hashPath(key)+"/"+url.PathEscape(field), nil, &out)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return out.Value, true, nil
}

// HGetAll returns the whole hash at key.
func (s *KVService) HGetAll(key string) (map[string]string, error) {
	var out struct {
		Fields map[string]string `json:"fields"`
	}
	err := s.http.DoJSON(http.MethodGet, hashPath(key), nil, &out)
	if out.Fields == nil {
		out.Fields = map[string]string{}
	}
	return out.Fields, err
}

// HDel removes a field from the hash at key.
func (s *KVService) HDel(key, field string) (int, error) {
	var out struct {
		Removed int `json:"removed"`
	}
	err := s.http.DoJSON(http.MethodDelete, hashPath(key)+"/"+url.PathEscape(field), nil, &out)
	return out.Removed, err
}

// HKeys returns the field names in the hash at key.
func (s *KVService) HKeys(key string) ([]string, error) {
	var out struct {
		Keys []string `json:"keys"`
	}
	err := s.http.DoJSON(http.MethodGet, hashPath(key)+"/_keys", nil, &out)
	return out.Keys, err
}

// HLen returns the number of fields in the hash at key.
func (s *KVService) HLen(key string) (int, error) {
	var out struct {
		Length int `json:"length"`
	}
	err := s.http.DoJSON(http.MethodGet, hashPath(key)+"/_len", nil, &out)
	return out.Length, err
}

// HIncrBy atomically adds delta to the integer field in the hash at key.
func (s *KVService) HIncrBy(key, field string, delta int64) (int64, error) {
	var out struct {
		Value int64 `json:"value"`
	}
	err := s.http.DoJSON(http.MethodPost, hashPath(key)+"/"+url.PathEscape(field)+"/incr",
		map[string]interface{}{"delta": delta}, &out)
	return out.Value, err
}

// ---- List ----

func (s *KVService) lpushRpush(op, key string, values []string) (int, error) {
	var out struct {
		Length int `json:"length"`
	}
	err := s.http.DoJSON(http.MethodPost, listPath(key)+"/"+op,
		map[string]interface{}{"values": values}, &out)
	return out.Length, err
}

// LPush prepends values to the head of the list at key.
func (s *KVService) LPush(key string, values ...string) (int, error) {
	return s.lpushRpush("lpush", key, values)
}

// RPush appends values to the tail of the list at key.
func (s *KVService) RPush(key string, values ...string) (int, error) {
	return s.lpushRpush("rpush", key, values)
}

func (s *KVService) pop(op, key string) (string, bool, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := s.http.DoJSON(http.MethodPost, listPath(key)+"/"+op, nil, &out)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return out.Value, true, nil
}

// LPop removes and returns the head element of the list at key.
func (s *KVService) LPop(key string) (string, bool, error) { return s.pop("lpop", key) }

// RPop removes and returns the tail element of the list at key.
func (s *KVService) RPop(key string) (string, bool, error) { return s.pop("rpop", key) }

// LLen returns the length of the list at key.
func (s *KVService) LLen(key string) (int, error) {
	var out struct {
		Length int `json:"length"`
	}
	err := s.http.DoJSON(http.MethodGet, listPath(key)+"/len", nil, &out)
	return out.Length, err
}

// LIndex returns the element at index (negative counts from the tail).
func (s *KVService) LIndex(key string, index int) (string, bool, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := s.http.DoJSON(http.MethodGet, listPath(key)+"/index/"+strconv.Itoa(index), nil, &out)
	if err != nil {
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return out.Value, true, nil
}

// LRange returns elements between start and stop (inclusive, Redis-style).
func (s *KVService) LRange(key string, start, stop int) ([]string, error) {
	p := listPath(key) + "?start=" + strconv.Itoa(start) + "&stop=" + strconv.Itoa(stop)
	var out struct {
		Values []string `json:"values"`
	}
	err := s.http.DoJSON(http.MethodGet, p, nil, &out)
	return out.Values, err
}

// ---- Set ----

// SAdd adds members to the set at key; returns the number newly added.
func (s *KVService) SAdd(key string, members ...string) (int, error) {
	var out struct {
		Added int `json:"added"`
	}
	err := s.http.DoJSON(http.MethodPost, setPath(key)+"/add",
		map[string]interface{}{"members": members}, &out)
	return out.Added, err
}

// SRem removes members from the set at key; returns the number removed.
func (s *KVService) SRem(key string, members ...string) (int, error) {
	var out struct {
		Removed int `json:"removed"`
	}
	err := s.http.DoJSON(http.MethodPost, setPath(key)+"/rem",
		map[string]interface{}{"members": members}, &out)
	return out.Removed, err
}

// SMembers returns all members of the set at key.
func (s *KVService) SMembers(key string) ([]string, error) {
	var out struct {
		Members []string `json:"members"`
	}
	err := s.http.DoJSON(http.MethodGet, setPath(key), nil, &out)
	return out.Members, err
}

// SIsMember reports whether member is in the set at key.
func (s *KVService) SIsMember(key, member string) (bool, error) {
	var out struct {
		Member bool `json:"member"`
	}
	err := s.http.DoJSON(http.MethodGet, setPath(key)+"/ismember/"+url.PathEscape(member), nil, &out)
	return out.Member, err
}

// SCard returns the number of members in the set at key.
func (s *KVService) SCard(key string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	err := s.http.DoJSON(http.MethodGet, setPath(key)+"/card", nil, &out)
	return out.Count, err
}
