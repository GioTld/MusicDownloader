package deezer

import (
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
