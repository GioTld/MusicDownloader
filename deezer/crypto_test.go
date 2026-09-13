package deezer

import (
	"bytes"
	"crypto/cipher"
	"math/rand"
	"testing"

	"golang.org/x/crypto/blowfish"
)

// encryptBF_CBC_STRIPE is a test helper that applies the reverse of decrypt()
// according to Deezer's BF_CBC_STRIPE format: every 3rd 2048-byte chunk is Blowfish-CBC encrypted.
func encryptBF_CBC_STRIPE(data, key []byte) ([]byte, error) {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		chunk := data[i:end]
		if (i/chunkSize)%encryptEvery == 0 && len(chunk) == chunkSize {
			enc := make([]byte, chunkSize)
			cipher.NewCBCEncrypter(block, blowfishIV).CryptBlocks(enc, chunk)
			out = append(out, enc...)
		} else {
			out = append(out, chunk...)
		}
	}
	return out, nil
}

func TestDeriveKey(t *testing.T) {
	tests := []struct {
		trackID string
	}{
		{"3135556"},
		{"12345678"},
		{"0"},
		{"9999999999"},
	}

	for _, tt := range tests {
		key := deriveKey(tt.trackID)
		if len(key) != 16 {
			t.Fatalf("expected key length 16, got %d for track %s", len(key), tt.trackID)
		}

		// Ensure determinism
		key2 := deriveKey(tt.trackID)
		if !bytes.Equal(key, key2) {
			t.Fatalf("deriveKey is not deterministic for track %s", tt.trackID)
		}
	}

	// Distinct track IDs should produce distinct keys
	k1 := deriveKey("100")
	k2 := deriveKey("200")
	if bytes.Equal(k1, k2) {
		t.Error("different track IDs produced identical keys")
	}
}

func TestDecryptRoundTrip(t *testing.T) {
	key := deriveKey("3135556")

	testSizes := []int{
		0,       // empty
		100,     // sub-chunk
		2048,    // exactly 1 chunk (encrypted)
		2049,    // 1 chunk + 1 byte
		4096,    // 2 chunks (1st encrypted, 2nd plaintext)
		6144,    // 3 chunks (chunk 0 encrypted, 1 & 2 plaintext)
		8192,    // 4 chunks (chunk 0 & 3 encrypted, 1 & 2 plaintext)
		100_000, // realistic size
	}

	for _, size := range testSizes {
		payload := make([]byte, size)
		rng := rand.New(rand.NewSource(int64(size)))
		rng.Read(payload)

		encrypted, err := encryptBF_CBC_STRIPE(payload, key)
		if err != nil {
			t.Fatalf("encrypt failed for size %d: %v", size, err)
		}

		decrypted, err := decrypt(encrypted, key)
		if err != nil {
			t.Fatalf("decrypt failed for size %d: %v", size, err)
		}

		if !bytes.Equal(payload, decrypted) {
			t.Fatalf("roundtrip mismatch for size %d", size)
		}
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	data := make([]byte, 2048)
	_, err := decrypt(data, []byte{})
	if err == nil {
		t.Error("expected error for empty key, got nil")
	}
}
