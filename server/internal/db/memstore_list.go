package db

// List operations — a key holding an ordered sequence of strings, like a Redis
// list. The head is index 0 (left), the tail is the last element (right).

func (m *MemStore) listForWrite(key string) (*Value, error) {
	if v, ok := m.getValue(key); ok {
		if v.Kind != KindList {
			return nil, wrongType(KindList, v.Kind)
		}
		return v.clone(), nil
	}
	return &Value{Kind: KindList, List: []string{}}, nil
}

func (m *MemStore) listForRead(key string) (*Value, bool, error) {
	v, ok := m.getValue(key)
	if !ok {
		return nil, false, nil
	}
	if v.Kind != KindList {
		return nil, false, wrongType(KindList, v.Kind)
	}
	return v, true, nil
}

// LPush prepends values to the head of the list (left). Returns the new length.
func (m *MemStore) LPush(key string, values ...string) (int, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	v, err := m.listForWrite(key)
	if err != nil {
		return 0, err
	}
	// prepend in order so the last arg ends up deepest, matching Redis LPUSH
	next := make([]string, 0, len(v.List)+len(values))
	for i := len(values) - 1; i >= 0; i-- {
		next = append(next, values[i])
	}
	next = append(next, v.List...)
	v.List = next
	m.setValue(key, v, m.remainingTTL(key))
	return len(v.List), nil
}

// RPush appends values to the tail of the list (right). Returns the new length.
func (m *MemStore) RPush(key string, values ...string) (int, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	v, err := m.listForWrite(key)
	if err != nil {
		return 0, err
	}
	v.List = append(v.List, values...)
	m.setValue(key, v, m.remainingTTL(key))
	return len(v.List), nil
}

// LPop removes and returns the head element. ok is false on an empty/missing list.
func (m *MemStore) LPop(key string) (string, bool, error) {
	return m.pop(key, true)
}

// RPop removes and returns the tail element.
func (m *MemStore) RPop(key string) (string, bool, error) {
	return m.pop(key, false)
}

func (m *MemStore) pop(key string, head bool) (string, bool, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	cur, ok, err := m.listForRead(key)
	if err != nil || !ok || len(cur.List) == 0 {
		return "", false, err
	}
	v := cur.clone()
	var out string
	if head {
		out = v.List[0]
		v.List = v.List[1:]
	} else {
		out = v.List[len(v.List)-1]
		v.List = v.List[:len(v.List)-1]
	}
	if len(v.List) == 0 {
		m.cache.Delete(key)
		m.markDirty()
		return out, true, nil
	}
	m.setValue(key, v, m.remainingTTL(key))
	return out, true, nil
}

// LLen returns the length of the list at key.
func (m *MemStore) LLen(key string) (int, error) {
	v, ok, err := m.listForRead(key)
	if err != nil || !ok {
		return 0, err
	}
	return len(v.List), nil
}

// LIndex returns the element at index (negative counts from the tail).
func (m *MemStore) LIndex(key string, index int) (string, bool, error) {
	v, ok, err := m.listForRead(key)
	if err != nil || !ok {
		return "", false, err
	}
	i := index
	if i < 0 {
		i = len(v.List) + i
	}
	if i < 0 || i >= len(v.List) {
		return "", false, nil
	}
	return v.List[i], true, nil
}

// LRange returns the elements between start and stop (inclusive), with Redis-
// style negative indexing (-1 is the last element).
func (m *MemStore) LRange(key string, start, stop int) ([]string, error) {
	v, ok, err := m.listForRead(key)
	if err != nil || !ok {
		return []string{}, err
	}
	n := len(v.List)
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return []string{}, nil
	}
	out := make([]string, stop-start+1)
	copy(out, v.List[start:stop+1])
	return out, nil
}
