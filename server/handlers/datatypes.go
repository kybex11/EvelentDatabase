package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// pathSegs splits the part of the URL after prefix into URL-decoded segments.
func pathSegs(path, prefix string) []string {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, "/")
	for i := range parts {
		if dec, err := url.PathUnescape(parts[i]); err == nil {
			parts[i] = dec
		}
	}
	return parts
}

// writeTypedErr maps a WRONGTYPE engine error to 409, anything else to 500.
func writeTypedErr(w http.ResponseWriter, err error) {
	if strings.HasPrefix(err.Error(), "WRONGTYPE") {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Hash: /api/hash/<key>[/<field>[/incr]]
// ---------------------------------------------------------------------------

func HashHandler(w http.ResponseWriter, r *http.Request) {
	kv := database.KV()
	segs := pathSegs(r.URL.Path, "/api/hash/")
	if len(segs) == 0 {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	key := segs[0]

	switch {
	case len(segs) == 1 && r.Method == http.MethodGet:
		all, err := kv.HGetAll(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"fields": all})
	case len(segs) == 1 && r.Method == http.MethodPut:
		var req struct {
			Fields map[string]string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		added, err := kv.HSetMany(key, req.Fields)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"added": added})
	case len(segs) == 2 && segs[1] == "_keys" && r.Method == http.MethodGet:
		keys, err := kv.HKeys(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"keys": keys})
	case len(segs) == 2 && segs[1] == "_len" && r.Method == http.MethodGet:
		n, err := kv.HLen(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"length": n})
	case len(segs) == 2 && r.Method == http.MethodGet:
		val, exists, err := kv.HGet(key, segs[1])
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		if !exists {
			http.Error(w, "field not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"value": val})
	case len(segs) == 2 && r.Method == http.MethodPut:
		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		added, err := kv.HSet(key, segs[1], req.Value)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"added": added})
	case len(segs) == 2 && r.Method == http.MethodDelete:
		removed, err := kv.HDel(key, segs[1])
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"removed": removed})
	case len(segs) == 3 && segs[2] == "incr" && r.Method == http.MethodPost:
		var req struct {
			Delta int64 `json:"delta"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Delta == 0 {
			req.Delta = 1
		}
		val, err := kv.HIncrBy(key, segs[1], req.Delta)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"value": val})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// List: /api/list/<key>[/<op>...]
// ---------------------------------------------------------------------------

func ListHandler(w http.ResponseWriter, r *http.Request) {
	kv := database.KV()
	segs := pathSegs(r.URL.Path, "/api/list/")
	if len(segs) == 0 {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	key := segs[0]

	readValues := func() ([]string, error) {
		start := atoiDefault(r.URL.Query().Get("start"), 0)
		stop := atoiDefault(r.URL.Query().Get("stop"), -1)
		return kv.LRange(key, start, stop)
	}

	switch {
	case len(segs) == 1 && r.Method == http.MethodGet:
		vals, err := readValues()
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"values": vals})
	case len(segs) == 2 && segs[1] == "len" && r.Method == http.MethodGet:
		n, err := kv.LLen(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"length": n})
	case len(segs) == 3 && segs[1] == "index" && r.Method == http.MethodGet:
		idx := atoiDefault(segs[2], 0)
		val, ok, err := kv.LIndex(key, idx)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		if !ok {
			http.Error(w, "index out of range", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"value": val})
	case len(segs) == 2 && (segs[1] == "lpush" || segs[1] == "rpush") && r.Method == http.MethodPost:
		var req struct {
			Values []string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var n int
		var err error
		if segs[1] == "lpush" {
			n, err = kv.LPush(key, req.Values...)
		} else {
			n, err = kv.RPush(key, req.Values...)
		}
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"length": n})
	case len(segs) == 2 && (segs[1] == "lpop" || segs[1] == "rpop") && r.Method == http.MethodPost:
		var val string
		var ok bool
		var err error
		if segs[1] == "lpop" {
			val, ok, err = kv.LPop(key)
		} else {
			val, ok, err = kv.RPop(key)
		}
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		if !ok {
			http.Error(w, "list empty", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{"value": val})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Set: /api/set/<key>[/<op>...]
// ---------------------------------------------------------------------------

func SetHandler(w http.ResponseWriter, r *http.Request) {
	kv := database.KV()
	segs := pathSegs(r.URL.Path, "/api/set/")
	if len(segs) == 0 {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	key := segs[0]

	switch {
	case len(segs) == 1 && r.Method == http.MethodGet:
		members, err := kv.SMembers(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"members": members})
	case len(segs) == 2 && segs[1] == "card" && r.Method == http.MethodGet:
		n, err := kv.SCard(key)
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"count": n})
	case len(segs) == 3 && segs[1] == "ismember" && r.Method == http.MethodGet:
		ok, err := kv.SIsMember(key, segs[2])
		if err != nil {
			writeTypedErr(w, err)
			return
		}
		writeJSON(w, map[string]interface{}{"member": ok})
	case len(segs) == 2 && (segs[1] == "add" || segs[1] == "rem") && r.Method == http.MethodPost:
		var req struct {
			Members []string `json:"members"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var n int
		var err error
		if segs[1] == "add" {
			n, err = kv.SAdd(key, req.Members...)
			if err != nil {
				writeTypedErr(w, err)
				return
			}
			writeJSON(w, map[string]interface{}{"added": n})
		} else {
			n, err = kv.SRem(key, req.Members...)
			if err != nil {
				writeTypedErr(w, err)
				return
			}
			writeJSON(w, map[string]interface{}{"removed": n})
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
