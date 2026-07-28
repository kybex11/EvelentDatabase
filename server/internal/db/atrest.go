package db

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// aeadCache reuses AES-GCM instances per 32-byte key so hot paths skip
// aes.NewCipher + cipher.NewGCM on every document encode/decode.
var aeadCache sync.Map // string(key) → cipher.AEAD

var noncePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 12) // AES-GCM standard nonce size
		return &b
	},
}

func EnsureEncryptionKey(rootDir string) ([]byte, error) {
	if s := os.Getenv("DB_ENCRYPTION_KEY"); s != "" {
		key, err := hex.DecodeString(s)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("invalid DB_ENCRYPTION_KEY")
		}
		return key, nil
	}
	keyPath := filepath.Join(rootDir, ".key")
	if data, err := os.ReadFile(keyPath); err == nil && len(data) == 32 {
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	tmp := keyPath + ".tmp"
	if err := os.WriteFile(tmp, key, 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, keyPath); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return key, nil
}

func looksLikeJSON(data []byte) bool {
	s := bytes.TrimSpace(data)
	if len(s) == 0 {
		return false
	}
	return s[0] == '{' || s[0] == '['
}

func getAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("missing encryption key")
	}
	cacheKey := string(key)
	if v, ok := aeadCache.Load(cacheKey); ok {
		return v.(cipher.AEAD), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	actual, _ := aeadCache.LoadOrStore(cacheKey, gcm)
	return actual.(cipher.AEAD), nil
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	gcm, err := getAEAD(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	noncePtr := noncePool.Get().(*[]byte)
	nonce := *noncePtr
	if cap(nonce) < ns {
		nonce = make([]byte, ns)
		*noncePtr = nonce
	}
	nonce = nonce[:ns]
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		noncePool.Put(noncePtr)
		return nil, err
	}
	out := gcm.Seal(nil, nonce, plaintext, nil)
	// Prepend nonce: [nonce|ciphertext+tag]
	blob := make([]byte, 0, ns+len(out))
	blob = append(blob, nonce...)
	blob = append(blob, out...)
	noncePool.Put(noncePtr)
	return blob, nil
}

func decryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	gcm, err := getAEAD(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	return gcm.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
}

func EncodePayload(encKey []byte, plain []byte) ([]byte, error) {
	if len(encKey) != 32 {
		return nil, fmt.Errorf("missing encryption key")
	}
	return encryptAESGCM(encKey, plain)
}

func DecodePayload(encKey []byte, data []byte) ([]byte, error) {
	if len(encKey) != 32 {
		return data, nil
	}
	if plain, err := decryptAESGCM(encKey, data); err == nil {
		return plain, nil
	}
	if looksLikeJSON(data) {
		return data, nil
	}
	return nil, fmt.Errorf("decrypt failed")
}

func encodeDocument(encKey []byte, doc map[string]interface{}) ([]byte, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return EncodePayload(encKey, data)
}

func decodeDocument(encKey []byte, data []byte, dest *map[string]interface{}) error {
	payload, err := DecodePayload(encKey, data)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".w-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
