package db

func MatchesFilter(doc map[string]interface{}, filter map[string]interface{}) bool {
	for key, cond := range filter {
		if condMap, ok := cond.(map[string]interface{}); ok {
			for op, val := range condMap {
				docVal, exists := doc[key]
				switch op {
				case "$eq":
					if !exists || !valuesEqual(docVal, val) {
						return false
					}
				case "$ne":
					if exists && valuesEqual(docVal, val) {
						return false
					}
				case "$gt":
					if !exists || compare(docVal, val) != 1 {
						return false
					}
				case "$gte":
					if !exists || compare(docVal, val) < 0 {
						return false
					}
				case "$lt":
					if !exists || compare(docVal, val) != -1 {
						return false
					}
				case "$lte":
					if !exists || compare(docVal, val) > 0 {
						return false
					}
				case "$in":
					if !exists {
						return false
					}
					arr, ok := val.([]interface{})
					if !ok {
						return false
					}
					found := false
					for _, v := range arr {
						if valuesEqual(docVal, v) {
							found = true
							break
						}
					}
					if !found {
						return false
					}
				default:
					return false
				}
			}
		} else {
			docVal, exists := doc[key]
			if !exists || !valuesEqual(docVal, cond) {
				return false
			}
		}
	}
	return true
}

// valuesEqual is a fast path for JSON-decoded scalars; falls back carefully
// without reflect.DeepEqual for the common cases.
func valuesEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case float64:
		if bv, ok := toFloat64(b); ok {
			return av == bv
		}
		return false
	case int:
		if bv, ok := toFloat64(b); ok {
			return float64(av) == bv
		}
		return false
	case int64:
		if bv, ok := toFloat64(b); ok {
			return float64(av) == bv
		}
		return false
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !valuesEqual(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func CompareValues(a, b interface{}) int {
	return compare(a, b)
}

func compare(a, b interface{}) int {
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)
	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1
		} else if aNum > bNum {
			return 1
		}
		return 0
	}
	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr && bIsStr {
		if aStr < bStr {
			return -1
		} else if aStr > bStr {
			return 1
		}
		return 0
	}
	return 0
}

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}
