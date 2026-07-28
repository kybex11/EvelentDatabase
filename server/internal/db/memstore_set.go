package db

// Set operations — a key holding an unordered collection of unique strings,
// like a Redis set.

func (m *MemStore) setForWrite(key string) (*Value, error) {
	if v, ok := m.getValue(key); ok {
		if v.Kind != KindSet {
			return nil, wrongType(KindSet, v.Kind)
		}
		return v.clone(), nil
	}
	return &Value{Kind: KindSet, Set: map[string]struct{}{}}, nil
}

func (m *MemStore) setForRead(key string) (*Value, bool, error) {
	v, ok := m.getValue(key)
	if !ok {
		return nil, false, nil
	}
	if v.Kind != KindSet {
		return nil, false, wrongType(KindSet, v.Kind)
	}
	return v, true, nil
}

// SAdd adds members to the set at key and returns how many were newly added.
func (m *MemStore) SAdd(key string, members ...string) (int, error) {
	unlock := m.lockKey(key)
	defer unlock()
	v, err := m.setForWrite(key)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, mem := range members {
		if _, exists := v.Set[mem]; !exists {
			v.Set[mem] = struct{}{}
			added++
		}
	}
	m.setValue(key, v, m.remainingTTL(key))
	return added, nil
}

// SRem removes members from the set and returns how many were removed. The key
// is deleted when the last member is removed.
func (m *MemStore) SRem(key string, members ...string) (int, error) {
	unlock := m.lockKey(key)
	defer unlock()
	cur, ok, err := m.setForRead(key)
	if err != nil || !ok {
		return 0, err
	}
	v := cur.clone()
	removed := 0
	for _, mem := range members {
		if _, exists := v.Set[mem]; exists {
			delete(v.Set, mem)
			removed++
		}
	}
	if len(v.Set) == 0 {
		m.cache.Delete(key)
		m.markDirty()
		return removed, nil
	}
	m.setValue(key, v, m.remainingTTL(key))
	return removed, nil
}

// SMembers returns all members of the set at key.
func (m *MemStore) SMembers(key string) ([]string, error) {
	v, ok, err := m.setForRead(key)
	if err != nil || !ok {
		return []string{}, err
	}
	out := make([]string, 0, len(v.Set))
	for mem := range v.Set {
		out = append(out, mem)
	}
	return out, nil
}

// SIsMember reports whether member is in the set at key.
func (m *MemStore) SIsMember(key, member string) (bool, error) {
	v, ok, err := m.setForRead(key)
	if err != nil || !ok {
		return false, err
	}
	_, exists := v.Set[member]
	return exists, nil
}

// SCard returns the number of members in the set at key.
func (m *MemStore) SCard(key string) (int, error) {
	v, ok, err := m.setForRead(key)
	if err != nil || !ok {
		return 0, err
	}
	return len(v.Set), nil
}
