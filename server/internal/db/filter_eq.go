package db

func EqualityIndexFilter(filter map[string]interface{}) (field string, value interface{}, ok bool) {
	if len(filter) != 1 {
		return "", nil, false
	}
	for k, v := range filter {
		if m, ok := v.(map[string]interface{}); ok {
			if len(m) != 1 {
				return "", nil, false
			}
			if eq, ok := m["$eq"]; ok {
				return k, eq, true
			}
			return "", nil, false
		}
		return k, v, true
	}
	return "", nil, false
}
