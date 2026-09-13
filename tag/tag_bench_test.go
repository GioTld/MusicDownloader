package tag

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkTagMP3(b *testing.B) {
	tmpDir := b.TempDir()
	sourceFile := filepath.Join(tmpDir, "source.mp3")
	dummyAudio := append([]byte{0xFF, 0xFB, 0x90, 0x64}, bytes.Repeat([]byte{0x55}, 128*1024)...) // 128KB dummy audio
	if err := os.WriteFile(sourceFile, dummyAudio, 0644); err != nil {
		b.Fatalf("failed to create source file: %v", err)
	}

	dummyCover := bytes.Repeat([]byte{0xAA}, 50*1024) // 50KB dummy cover art
	info := TrackInfo{
		Title:       "Benchmark Track",
		Artist:      "Benchmark Artist",
		Album:       "Benchmark Album",
		TrackNumber: 5,
		DiscNumber:  1,
		Year:        "2025",
		Lyrics:      "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	targetPath := filepath.Join(tmpDir, "target.mp3")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(targetPath, dummyAudio, 0644)
		b.StartTimer()

		if err := TagMP3(targetPath, info, dummyCover); err != nil {
			b.Fatalf("TagMP3 failed: %v", err)
		}
	}
}

func BenchmarkTagFLAC(b *testing.B) {
	tmpDir := b.TempDir()
	var buf bytes.Buffer
	buf.Write([]byte("fLaC"))
	buf.Write([]byte{0x80, 0x00, 0x00, 0x22})
	buf.Write(make([]byte, 34)) // STREAMINFO
	buf.Write(bytes.Repeat([]byte{0x55}, 128*1024))
	dummyFLAC := buf.Bytes()

	dummyCover := bytes.Repeat([]byte{0xAA}, 50*1024)
	info := TrackInfo{
		Title:       "Benchmark Track",
		Artist:      "Benchmark Artist",
		AlbumArtist: "Benchmark Album Artist",
		Album:       "Benchmark Album",
		Genre:       "Rock",
		ISRC:        "USRC17607839",
		TrackNumber: 5,
		TrackTotal:  12,
		DiscNumber:  1,
		DiscTotal:   1,
		Year:        "2025",
		Lyrics:      "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}

	targetPath := filepath.Join(tmpDir, "target.flac")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		_ = os.WriteFile(targetPath, dummyFLAC, 0644)
		b.StartTimer()

		if err := TagFLAC(targetPath, info, dummyCover); err != nil {
			b.Fatalf("TagFLAC failed: %v", err)
		}
	}
}
