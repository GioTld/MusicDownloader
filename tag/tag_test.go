package tag

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	id3 "github.com/bogem/id3v2/v2"
)

// createDummyMP3 writes a minimal MP3-like file to filePath.
func createDummyMP3(t *testing.T, filePath string) {
	t.Helper()
	// Dummy audio payload (MPEG-1 Layer 3 sync header + padding)
	dummyData := append([]byte{0xFF, 0xFB, 0x90, 0x64}, bytes.Repeat([]byte{0x00}, 1024)...)
	if err := os.WriteFile(filePath, dummyData, 0644); err != nil {
		t.Fatalf("failed to create dummy MP3: %v", err)
	}
}

func TestTagMP3Full(t *testing.T) {
	tmpDir := t.TempDir()
	mp3Path := filepath.Join(tmpDir, "test.mp3")
	createDummyMP3(t, mp3Path)

	dummyCover := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	info := TrackInfo{
		Title:       "Bohemian Rhapsody",
		Artist:      "Queen",
		Album:       "A Night at the Opera",
		TrackNumber: 11,
		DiscNumber:  2,
		Year:        "1975",
		Lyrics:      "Is this the real life? Is this just fantasy?",
	}

	err := TagMP3(mp3Path, info, dummyCover)
	if err != nil {
		t.Fatalf("TagMP3 failed: %v", err)
	}

	// Verify the written tags by parsing the file with id3v2
	f, err := id3.Open(mp3Path, id3.Options{Parse: true})
	if err != nil {
		t.Fatalf("failed to open tagged MP3: %v", err)
	}
	defer f.Close()

	if got := f.Title(); got != info.Title {
		t.Errorf("Title = %q, want %q", got, info.Title)
	}
	if got := f.Artist(); got != info.Artist {
		t.Errorf("Artist = %q, want %q", got, info.Artist)
	}
	if got := f.Album(); got != info.Album {
		t.Errorf("Album = %q, want %q", got, info.Album)
	}
	if got := f.Year(); got != info.Year {
		t.Errorf("Year = %q, want %q", got, info.Year)
	}

	// Verify track and disc numbers
	trckFrames := f.GetFrames("TRCK")
	if len(trckFrames) == 0 {
		t.Error("missing TRCK frame")
	} else if tf, ok := trckFrames[0].(id3.TextFrame); !ok || tf.Text != "11" {
		t.Errorf("TRCK frame text = %v, want 11", trckFrames[0])
	}

	tposFrames := f.GetFrames("TPOS")
	if len(tposFrames) == 0 {
		t.Error("missing TPOS frame")
	} else if tf, ok := tposFrames[0].(id3.TextFrame); !ok || tf.Text != "2" {
		t.Errorf("TPOS frame text = %v, want 2", tposFrames[0])
	}

	// Verify lyrics
	usltFrames := f.GetFrames(f.CommonID("Unsynchronised lyrics/text transcription"))
	if len(usltFrames) == 0 {
		t.Error("missing USLT (lyrics) frame")
	} else if lf, ok := usltFrames[0].(id3.UnsynchronisedLyricsFrame); !ok || lf.Lyrics != info.Lyrics {
		t.Errorf("lyrics = %q, want %q", lf.Lyrics, info.Lyrics)
	}

	// Verify attached picture
	picFrames := f.GetFrames(f.CommonID("Attached picture"))
	if len(picFrames) == 0 {
		t.Error("missing APIC (cover) frame")
	} else if pf, ok := picFrames[0].(id3.PictureFrame); !ok || !bytes.Equal(pf.Picture, dummyCover) {
		t.Error("attached picture content does not match dummy cover")
	}
}

func TestTagMP3Minimal(t *testing.T) {
	tmpDir := t.TempDir()
	mp3Path := filepath.Join(tmpDir, "minimal.mp3")
	createDummyMP3(t, mp3Path)

	info := TrackInfo{
		Title:  "Simple Song",
		Artist: "Simple Artist",
	}

	err := TagMP3(mp3Path, info, nil)
	if err != nil {
		t.Fatalf("TagMP3 failed: %v", err)
	}

	f, err := id3.Open(mp3Path, id3.Options{Parse: true})
	if err != nil {
		t.Fatalf("failed to open tagged MP3: %v", err)
	}
	defer f.Close()

	if got := f.Title(); got != info.Title {
		t.Errorf("Title = %q, want %q", got, info.Title)
	}
	if got := f.Artist(); got != info.Artist {
		t.Errorf("Artist = %q, want %q", got, info.Artist)
	}
	// Disc number 0/1 shouldn't add TPOS
	if len(f.GetFrames("TPOS")) > 0 {
		t.Error("expected no TPOS frame when DiscNumber <= 1")
	}
}

func TestTagMP3NonExistentFile(t *testing.T) {
	err := TagMP3("/nonexistent/path/song.mp3", TrackInfo{Title: "Test"}, nil)
	if err == nil {
		t.Error("expected error when tagging non-existent file, got nil")
	}
}
