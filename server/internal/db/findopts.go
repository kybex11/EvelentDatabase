package db

import (
	"encoding/base64"
	"encoding/json"
)

type FindOptions struct {
	Limit       int
	Skip        int
	SortField   string
	SortDesc    bool
	Projection  map[string]interface{}
	CursorSkip  int
	ExplicitLim bool
}

func ParseFindRequest(m map[string]interface{}) (map[string]interface{}, *FindOptions) {
	opt := &FindOptions{Limit: -1}
	_, hasFilter := m["filter"]
	_, hasLimit := m["limit"]
	_, hasSkip := m["skip"]
	_, hasSort := m["sort"]
	_, hasProj := m["projection"]
	_, hasCursor := m["cursor"]
	if !hasFilter && !hasLimit && !hasSkip && !hasSort && !hasProj && !hasCursor {
		return m, opt
	}
	var filter map[string]interface{}
	if f, ok := m["filter"].(map[string]interface{}); ok {
		filter = f
	} else {
		filter = map[string]interface{}{}
	}
	if v, ok := m["limit"].(float64); ok {
		opt.Limit = int(v)
		opt.ExplicitLim = true
	}
	if v, ok := m["limit"].(int); ok {
		opt.Limit = v
		opt.ExplicitLim = true
	}
	if v, ok := m["skip"].(float64); ok {
		opt.Skip = int(v)
	}
	if v, ok := m["skip"].(int); ok {
		opt.Skip = v
	}
	if sm, ok := m["sort"].(map[string]interface{}); ok {
		if f, ok := sm["field"].(string); ok {
			opt.SortField = f
		}
		if o, ok := sm["order"].(float64); ok {
			opt.SortDesc = o < 0
		}
	}
	if pm, ok := m["projection"].(map[string]interface{}); ok {
		opt.Projection = pm
	}
	if cs, ok := m["cursor"].(string); ok && cs != "" {
		if s := decodeCursorSkip(cs); s >= 0 {
			opt.CursorSkip = s
			opt.Skip = s
		}
	}
	if !opt.ExplicitLim {
		opt.Limit = DefaultFindLimit
	}
	if opt.Limit < 0 {
		opt.Limit = DefaultFindLimit
	}
	if opt.Limit > MaxFindLimit {
		opt.Limit = MaxFindLimit
	}
	return filter, opt
}

func decodeCursorSkip(s string) int {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return -1
	}
	var p struct {
		Skip int `json:"skip"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return -1
	}
	return p.Skip
}

func EncodeCursorSkip(skip int) string {
	b, _ := json.Marshal(map[string]int{"skip": skip})
	return base64.RawURLEncoding.EncodeToString(b)
}
