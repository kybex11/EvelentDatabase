package db

// RangeSpec describes a single-field range filter that can use a sorted index.
type RangeSpec struct {
	Field  string
	Lo     interface{} // nil = open
	Hi     interface{} // nil = open
	LoIncl bool
	HiIncl bool
}

// RangeIndexFilter detects filters like {age:{$gte:18,$lt:65}} on exactly one
// field with only range operators. Equality-only filters are handled elsewhere.
func RangeIndexFilter(filter map[string]interface{}) (RangeSpec, bool) {
	if len(filter) != 1 {
		return RangeSpec{}, false
	}
	for field, cond := range filter {
		m, ok := cond.(map[string]interface{})
		if !ok || len(m) == 0 {
			return RangeSpec{}, false
		}
		spec := RangeSpec{Field: field, LoIncl: true, HiIncl: true}
		hasRange := false
		for op, val := range m {
			switch op {
			case "$gt":
				spec.Lo, spec.LoIncl = val, false
				hasRange = true
			case "$gte":
				spec.Lo, spec.LoIncl = val, true
				hasRange = true
			case "$lt":
				spec.Hi, spec.HiIncl = val, false
				hasRange = true
			case "$lte":
				spec.Hi, spec.HiIncl = val, true
				hasRange = true
			case "$eq":
				// Not a range — let equality path handle pure $eq.
				return RangeSpec{}, false
			default:
				return RangeSpec{}, false
			}
		}
		if !hasRange || (spec.Lo == nil && spec.Hi == nil) {
			return RangeSpec{}, false
		}
		return spec, true
	}
	return RangeSpec{}, false
}
