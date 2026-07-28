package db

import (
	"encoding/binary"
	"math"
)

// Fixed-size on-disk DocLoc encoding (17 bytes).
const docLocWireSize = 17

func encodeDocLoc(loc DocLoc) []byte {
	b := make([]byte, docLocWireSize)
	binary.BigEndian.PutUint32(b[0:4], loc.Seg)
	binary.BigEndian.PutUint64(b[4:12], uint64(loc.Offset))
	binary.BigEndian.PutUint32(b[12:16], loc.Length)
	if loc.Dead {
		b[16] = 1
	}
	return b
}

func decodeDocLoc(b []byte) (DocLoc, bool) {
	if len(b) < docLocWireSize {
		return DocLoc{}, false
	}
	loc := DocLoc{
		Seg:    binary.BigEndian.Uint32(b[0:4]),
		Offset: int64(binary.BigEndian.Uint64(b[4:12])),
		Length: binary.BigEndian.Uint32(b[12:16]),
		Dead:   b[16] != 0,
	}
	return loc, true
}

// encodeFloatSortable produces an 8-byte key that sorts in the same order as float64.
func encodeFloatSortable(f float64) []byte {
	u := math.Float64bits(f)
	if u&(1<<63) != 0 {
		u = ^u
	} else {
		u |= 1 << 63
	}
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, u)
	return b
}

func decodeFloatSortable(b []byte) (float64, bool) {
	if len(b) < 8 {
		return 0, false
	}
	u := binary.BigEndian.Uint64(b[:8])
	if u&(1<<63) != 0 {
		u &= ^uint64(1 << 63)
	} else {
		u = ^u
	}
	return math.Float64frombits(u), true
}

func encodeInt64(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func decodeInt64(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b[:8]))
}
