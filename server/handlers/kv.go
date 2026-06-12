package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"db/server/internal/db"
)

// kvKeyFromPath extracts the (URL-decoded) key from /api/kv/<key>[/op].
func kvKeyFromPath(path string) (key, op string) {
	rest := strings.TrimPrefix(path, "/api/kv/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return "", ""
	}
	// recognised trailing sub-operations
	for _, suffix := range []string{"/incr", "/expire", "/ttl"} {
		if strings.HasSuffix(rest, suffix) {
			raw := strings.TrimSuffix(rest, suffix)
			k, _ := url.PathUnescape(raw)
			return k, strings.TrimPrefix(suffix, "/")
		}
	}
	k, _ := url.PathUnescape(rest)
	return k, ""
}

func ttlFromSeconds(sec float64) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

// KVCollectionHandler serves the collection-level routes:
//
//	GET    /api/kv/keys?prefix=...   list keys
//	GET    /api/kv/stats             cache statistics
//	POST   /api/kv/flush             drop every key
//	POST   /api/kv/mget              batch read   {"keys":[...]}
//	POST   /api/kv/mset              batch write  {"items":[{key,value}], "ttlSeconds":N}
//	POST   /api/kv/mdel              batch delete {"keys":[...]}
func KVCollectionHandler(w http.ResponseWriter, r *http.Request) {
	kv := database.KV()
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/keys"):
		prefix := r.URL.Query().Get("prefix")
		writeJSON(w, map[string]interface{}{"keys": kv.Keys(prefix)})
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/stats"):
		writeJSON(w, kv.Stats())
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/flush"):
		kv.Flush()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/mget"):
		var req struct {
			Keys []string `json:"keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		items, missing := kv.MGet(req.Keys)
		if items == nil {
			items = []db.KVItem{}
		}
		if missing == nil {
			missing = []string{}
		}
		writeJSON(w, map[string]interface{}{"items": items, "missing": missing})
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/mset"):
		var req struct {
			Items      []db.KVItem `json:"items"`
			TTLSeconds float64     `json:"ttlSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n := kv.MSet(req.Items, ttlFromSeconds(req.TTLSeconds))
		writeJSON(w, map[string]interface{}{"written": n})
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/mdel"):
		var req struct {
			Keys []string `json:"keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]interface{}{"deleted": kv.DeleteMany(req.Keys)})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// KVKeyHandler serves the per-key routes:
//
//	GET    /api/kv/<key>          read     -> {value} or 404
//	PUT    /api/kv/<key>          write    body {"value":..,"ttlSeconds":N,"nx":bool}
//	DELETE /api/kv/<key>          delete
//	GET    /api/kv/<key>/ttl      remaining ttl
//	POST   /api/kv/<key>/incr     body {"delta":N}
//	POST   /api/kv/<key>/expire   body {"ttlSeconds":N}
func KVKeyHandler(w http.ResponseWriter, r *http.Request) {
	kv := database.KV()
	key, op := kvKeyFromPath(r.URL.Path)
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	switch op {
	case "ttl":
		remaining, persists, ok := kv.TTL(key)
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{
			"persists":   persists,
			"ttlSeconds": remaining.Seconds(),
		})
		return
	case "incr":
		var req struct {
			Delta int64 `json:"delta"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Delta == 0 {
			req.Delta = 1
		}
		val, err := kv.Incr(key, req.Delta)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]interface{}{"value": val})
		return
	case "expire":
		var req struct {
			TTLSeconds float64 `json:"ttlSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ok := kv.Expire(key, ttlFromSeconds(req.TTLSeconds))
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodGet:
		v, ok := kv.Get(key)
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"key": key, "value": v})
	case http.MethodPut:
		var req struct {
			Value      string  `json:"value"`
			TTLSeconds float64 `json:"ttlSeconds"`
			NX         bool    `json:"nx"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ttl := ttlFromSeconds(req.TTLSeconds)
		if req.NX {
			if !kv.SetNX(key, req.Value, ttl) {
				http.Error(w, "key already exists", http.StatusConflict)
				return
			}
		} else {
			kv.Set(key, req.Value, ttl)
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if !kv.Delete(key) {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
