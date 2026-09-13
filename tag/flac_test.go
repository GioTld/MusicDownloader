package tag

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createDummyFLAC creates a minimal valid FLAC file with a STREAMINFO block.
func createDummyFLAC(t *testing.T, filePath string) {
	t.Helper()

	var buf bytes.Buffer
	buf.Write([]byte("fLaC"))

	// STREAMINFO metadata block (type 0, 34 bytes)
	// Header: isLast = 1 (0x80 | 0x00), length = 34 (0x00, 0x00, 0x22)
	buf.Write([]byte{0x80, 0x00, 0x00, 0x22})
	buf.Write(make([]byte, 34)) // 34 bytes of zeroed STREAMINFO

	// Dummy audio frame bytes
	buf.Write([]byte{0xFF, 0xF8, 0x69, 0x02, 0x00, 0x11, 0x22, 0x33})

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to create dummy FLAC: %v", err)
	}
}

func TestTagFLACAndTagFile(t *testing.T) {
	tmpDir := t.TempDir()
	flacPath := filepath.Join(tmpDir, "test.flac")
	createDummyFLAC(t, flacPath)

	dummyCover := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	info := TrackInfo{
		Title:       "Shine On You Crazy Diamond",
		Artist:      "Pink Floyd",
		AlbumArtist: "Pink Floyd",
		Album:       "Wish You Were Here",
		Genre:       "Progressive Rock",
		ISRC:        "GBAYE7500010",
		TrackNumber: 1,
		TrackTotal:  5,
		DiscNumber:  1,
		DiscTotal:   1,
		Year:        "1975",
		Lyrics:      "Remember when you were young...",
	}

	// Use TagFile to test dispatching to TagFLAC
	if err := TagFile(flacPath, info, dummyCover); err != nil {
		t.Fatalf("TagFile (.flac) failed: %v", err)
	}

	// Verify FLAC file structure
	f, err := os.Open(flacPath)
	if err != nil {
		t.Fatalf("failed to open tagged FLAC: %v", err)
	}
	defer f.Close()

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil || string(magic[:]) != "fLaC" {
		t.Fatalf("invalid FLAC magic: %v", magic)
	}

	foundVorbis := false
	foundPicture := false
	var vorbisData []byte
	var pictureData []byte

	for {
		var hdr [4]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			t.Fatalf("failed reading block header: %v", err)
		}
		isLast := (hdr[0] & 0x80) != 0
		blockType := hdr[0] & 0x7F
		length := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])

		data := make([]byte, length)
		if _, err := io.ReadFull(f, data); err != nil {
			t.Fatalf("failed reading block data: %v", err)
		}

		if blockType == blockVorbisComment {
			foundVorbis = true
			vorbisData = data
		}
		if blockType == blockPicture {
			foundPicture = true
			pictureData = data
		}

		if isLast {
			break
		}
	}

	if !foundVorbis {
		t.Fatal("missing VORBIS_COMMENT block in tagged FLAC")
	}
	if !foundPicture {
		t.Fatal("missing PICTURE block in tagged FLAC")
	}

	// Parse Vorbis Comments
	r := bytes.NewReader(vorbisData)
	var vendorLen uint32
	_ = binary.Read(r, binary.LittleEndian, &vendorLen)
	vendor := make([]byte, vendorLen)
	_ = binary.Read(r, binary.LittleEndian, &vendor)
	if string(vendor) != "MusicDownloader" {
		t.Errorf("vendor = %q, want MusicDownloader", string(vendor))
	}

	var commentCount uint32
	_ = binary.Read(r, binary.LittleEndian, &commentCount)

	comments := make(map[string]string)
	for range commentCount {
		var cLen uint32
		_ = binary.Read(r, binary.LittleEndian, &cLen)
		cBytes := make([]byte, cLen)
		_ = binary.Read(r, binary.LittleEndian, &cBytes)
		parts := strings.SplitN(string(cBytes), "=", 2)
		if len(parts) == 2 {
			comments[parts[0]] = parts[1]
		}
	}

	expected := map[string]string{
		"TITLE":       info.Title,
		"ARTIST":      info.Artist,
		"ALBUM":       info.Album,
		"ALBUMARTIST": info.AlbumArtist,
		"GENRE":       info.Genre,
		"ISRC":        info.ISRC,
		"DATE":        info.Year,
		"TRACKNUMBER": "1",
		"TRACKTOTAL":  "5",
		"DISCNUMBER":  "1",
		"DISCTOTAL":   "1",
		"LYRICS":      info.Lyrics,
	}

	for k, v := range expected {
		if got := comments[k]; got != v {
			t.Errorf("vorbis comment %s = %q, want %q", k, got, v)
		}
	}

	// Verify Picture Block
	pr := bytes.NewReader(pictureData)
	var picType uint32
	_ = binary.Read(pr, binary.BigEndian, &picType)
	if picType != 3 {
		t.Errorf("picture type = %d, want 3 (front cover)", picType)
	}

	var mimeLen uint32
	_ = binary.Read(pr, binary.BigEndian, &mimeLen)
	mime := make([]byte, mimeLen)
	_ = binary.Read(pr, binary.BigEndian, &mime)
	if string(mime) != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", string(mime))
	}

	// Skip description, width, height, depth, colors
	var descLen uint32
	_ = binary.Read(pr, binary.BigEndian, &descLen)
	_, _ = pr.Seek(int64(descLen)+16, io.SeekCurrent)

	var dataLen uint32
	_ = binary.Read(pr, binary.BigEndian, &dataLen)
	picBytes := make([]byte, dataLen)
	_, _ = io.ReadFull(pr, picBytes)

	if !bytes.Equal(picBytes, dummyCover) {
		t.Error("picture bytes in FLAC block do not match input cover")
	}
}
