package db

import (
	"reflect"
)

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

func valuesEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
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
	case int64:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}
