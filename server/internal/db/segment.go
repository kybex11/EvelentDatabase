package db

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const MaxSegmentBytes int64 = 64 << 20 // 64 MiB

func segmentFileName(n uint32) string {
	return fmt.Sprintf("%06d.seg", n)
}

func segmentPath(segDir string, n uint32) string {
	return filepath.Join(segDir, segmentFileName(n))
}

// appendEncrypted appends a length-prefixed ciphertext record and returns the
// payload offset/length (not including the 4-byte length header).
func appendEncrypted(f *os.File, ciphertext []byte) (offset int64, length uint32, err error) {
	if len(ciphertext) == 0 {
		return 0, 0, fmt.Errorf("empty ciphertext")
	}
	if len(ciphertext) > int(^uint32(0)>>1) {
		return 0, 0, fmt.Errorf("record too large")
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0, err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(ciphertext)))
	if _, err := f.Write(hdr[:]); err != nil {
		return 0, 0, err
	}
	payloadOff := end + 4
	if _, err := f.Write(ciphertext); err != nil {
		return 0, 0, err
	}
	return payloadOff, uint32(len(ciphertext)), nil
}

func readAtSegment(path string, offset int64, length uint32) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, length)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	return buf, nil
}

func listSegmentNumbers(segDir string) ([]uint32, error) {
	entries, err := os.ReadDir(segDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var nums []uint32
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".seg") {
			continue
		}
		base := strings.TrimSuffix(name, ".seg")
		n, err := strconv.ParseUint(base, 10, 32)
		if err != nil {
			continue
		}
		nums = append(nums, uint32(n))
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	return nums, nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
