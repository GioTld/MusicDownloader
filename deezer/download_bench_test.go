package deezer

import (
	"os"
	"testing"
)

func BenchmarkSanitize(b *testing.B) {
	name := `AC/DC - Highway to Hell (Live in 1979: Remastered <Deluxe Edition>)?*`
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = sanitize(name)
	}
}

func BenchmarkAlbumFilename(b *testing.B) {
	track := Track{
		TrackNumber: 12,
		Title:       "Stairway to Heaven (Remaster)",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = albumFilename(track)
	}
}

func BenchmarkAlbumDirName(b *testing.B) {
	album := Album{
		Year:  "1971",
		Title: "Led Zeppelin IV",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = albumDirName(album)
	}
}

func BenchmarkValidateTrackFile(b *testing.B) {
	tmpDir := b.TempDir()
	targetPath := tmpDir + "/test.mp3"
	data := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), make([]byte, 120*1024)...)
	_ = os.WriteFile(targetPath, data, 0644)
	size := int64(len(data))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = validateTrackFile(targetPath, size)
	}
}
