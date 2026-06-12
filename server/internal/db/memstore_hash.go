package db

import "strconv"

// Hash operations — a key holding a map of field -> value, like a Redis hash.
// All mutations are atomic (guarded by opMu) and copy-on-write so the object
// held by the cache is never mutated in place.

func (m *MemStore) hashForWrite(key string) (*Value, error) {
	if v, ok := m.getValue(key); ok {
		if v.Kind != KindHash {
			return nil, wrongType(KindHash, v.Kind)
		}
		return v.clone(), nil
	}
	return &Value{Kind: KindHash, Hash: map[string]string{}}, nil
}

func (m *MemStore) hashForRead(key string) (*Value, bool, error) {
	v, ok := m.getValue(key)
	if !ok {
		return nil, false, nil
	}
	if v.Kind != KindHash {
		return nil, false, wrongType(KindHash, v.Kind)
	}
	return v, true, nil
}

// HSet sets field to value in the hash at key, creating the hash if needed.
// Returns the number of newly created fields (0 if the field already existed).
func (m *MemStore) HSet(key, field, value string) (int, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	v, err := m.hashForWrite(key)
	if err != nil {
		return 0, err
	}
	added := 0
	if _, exists := v.Hash[field]; !exists {
		added = 1
	}
	v.Hash[field] = value
	m.setValue(key, v, m.remainingTTL(key))
	return added, nil
}

// HSetMany sets multiple fields at once and returns the number of new fields.
func (m *MemStore) HSetMany(key string, fields map[string]string) (int, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	v, err := m.hashForWrite(key)
	if err != nil {
		return 0, err
	}
	added := 0
	for f, val := range fields {
		if _, exists := v.Hash[f]; !exists {
			added++
		}
		v.Hash[f] = val
	}
	m.setValue(key, v, m.remainingTTL(key))
	return added, nil
}

// HGet returns the value of field in the hash at key.
func (m *MemStore) HGet(key, field string) (string, bool, error) {
	v, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return "", false, err
	}
	val, exists := v.Hash[field]
	return val, exists, nil
}

// HGetAll returns a copy of the whole hash at key.
func (m *MemStore) HGetAll(key string) (map[string]string, error) {
	v, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return map[string]string{}, err
	}
	out := make(map[string]string, len(v.Hash))
	for f, val := range v.Hash {
		out[f] = val
	}
	return out, nil
}

// HDel removes fields from the hash and returns how many were removed. The key
// is deleted entirely when the last field is removed.
func (m *MemStore) HDel(key string, fields ...string) (int, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	cur, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return 0, err
	}
	v := cur.clone()
	removed := 0
	for _, f := range fields {
		if _, exists := v.Hash[f]; exists {
			delete(v.Hash, f)
			removed++
		}
	}
	if len(v.Hash) == 0 {
		m.cache.Delete(key)
		m.markDirty()
		return removed, nil
	}
	m.setValue(key, v, m.remainingTTL(key))
	return removed, nil
}

// HKeys returns the field names in the hash at key.
func (m *MemStore) HKeys(key string) ([]string, error) {
	v, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return []string{}, err
	}
	out := make([]string, 0, len(v.Hash))
	for f := range v.Hash {
		out = append(out, f)
	}
	return out, nil
}

// HLen returns the number of fields in the hash at key.
func (m *MemStore) HLen(key string) (int, error) {
	v, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return 0, err
	}
	return len(v.Hash), nil
}

// HExists reports whether field exists in the hash at key.
func (m *MemStore) HExists(key, field string) (bool, error) {
	v, ok, err := m.hashForRead(key)
	if err != nil || !ok {
		return false, err
	}
	_, exists := v.Hash[field]
	return exists, nil
}

// HIncrBy atomically adds delta to the integer field in the hash at key.
func (m *MemStore) HIncrBy(key, field string, delta int64) (int64, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	v, err := m.hashForWrite(key)
	if err != nil {
		return 0, err
	}
	cur := int64(0)
	if s, exists := v.Hash[field]; exists {
		n, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return 0, perr
		}
		cur = n
	}
	cur += delta
	v.Hash[field] = strconv.FormatInt(cur, 10)
	m.setValue(key, v, m.remainingTTL(key))
	return cur, nil
}
