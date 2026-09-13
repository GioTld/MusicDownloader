package deezer

import (
	"math/rand"
	"testing"
)

func BenchmarkDeriveKey(b *testing.B) {
	trackID := "3135556"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = deriveKey(trackID)
	}
}

func benchmarkDecryptSize(b *testing.B, size int) {
	key := deriveKey("3135556")
	payload := make([]byte, size)
	rng := rand.New(rand.NewSource(42))
	rng.Read(payload)

	encrypted, err := encryptBF_CBC_STRIPE(payload, key)
	if err != nil {
		b.Fatalf("encrypt failed: %v", err)
	}

	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = decrypt(encrypted, key)
	}
}

func BenchmarkDecrypt_1MB(b *testing.B) {
	benchmarkDecryptSize(b, 1024*1024)
}

func BenchmarkDecrypt_5MB(b *testing.B) {
	benchmarkDecryptSize(b, 5*1024*1024)
}

func BenchmarkDecrypt_10MB(b *testing.B) {
	benchmarkDecryptSize(b, 10*1024*1024)
}
