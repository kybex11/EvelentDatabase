package db

func ApplyProjection(doc map[string]interface{}, proj map[string]interface{}) map[string]interface{} {
	if len(proj) == 0 {
		return doc
	}
	out := make(map[string]interface{}, len(proj))
	for k, v := range proj {
		if iv, ok := v.(float64); ok && iv == 1 {
			if val, ok := doc[k]; ok {
				out[k] = val
			}
			continue
		}
		if b, ok := v.(bool); ok && b {
			if val, ok := doc[k]; ok {
				out[k] = val
			}
		}
	}
	if _, idok := out["_id"]; !idok {
		if id, ok := doc["_id"]; ok {
			out["_id"] = id
		}
	}
	return out
}
