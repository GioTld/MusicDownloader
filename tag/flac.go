package tag

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

var flacMagic = []byte("fLaC")

const (
	blockStreamInfo    = 0
	blockPadding       = 1
	blockApplication   = 2
	blockSeekTable     = 3
	blockVorbisComment = 4
	blockCueSheet      = 5
	blockPicture       = 6
)

type rawBlock struct {
	blockType byte
	data      []byte
}

// TagFLAC writes Vorbis comment tags and picture data to the FLAC file at filePath.
func TagFLAC(filePath string, info TrackInfo, cover []byte) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("tag flac: open %s: %w", filePath, err)
	}
	defer f.Close()

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return fmt.Errorf("tag flac: read header: %w", err)
	}
	if !bytes.Equal(magic[:], flacMagic) {
		return fmt.Errorf("tag flac: not a flac file: %s", filePath)
	}

	var streamInfo []byte
	var otherBlocks []rawBlock

	for {
		var header [4]byte
		if _, err := io.ReadFull(f, header[:]); err != nil {
			return fmt.Errorf("tag flac: read block header: %w", err)
		}

		isLast := (header[0] & 0x80) != 0
		blockType := header[0] & 0x7F
		length := int(header[1])<<16 | int(header[2])<<8 | int(header[3])

		data := make([]byte, length)
		if _, err := io.ReadFull(f, data); err != nil {
			return fmt.Errorf("tag flac: read block body: %w", err)
		}

		if blockType == blockStreamInfo {
			streamInfo = data
		} else if blockType != blockVorbisComment && blockType != blockPicture && blockType != blockPadding {
			otherBlocks = append(otherBlocks, rawBlock{blockType: blockType, data: data})
		}

		if isLast {
			break
		}
	}

	if streamInfo == nil {
		return errors.New("tag flac: missing STREAMINFO block")
	}

	// Build Vorbis Comment block
	vcData := buildVorbisComment(info)

	// Prepare blocks to write
	blocks := make([]rawBlock, 0, len(otherBlocks)+3)
	blocks = append(blocks, rawBlock{blockType: blockStreamInfo, data: streamInfo})
	blocks = append(blocks, otherBlocks...)
	blocks = append(blocks, rawBlock{blockType: blockVorbisComment, data: vcData})

	// Build Picture block if cover is provided
	if len(cover) > 0 {
		picData := buildPictureBlock(cover)
		blocks = append(blocks, rawBlock{blockType: blockPicture, data: picData})
	}

	// Create temp file in same directory
	tmpFile, err := os.CreateTemp(filepath.Dir(filePath), "tagflac-*.tmp")
	if err != nil {
		return fmt.Errorf("tag flac: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	// Write "fLaC"
	if _, err := tmpFile.Write(flacMagic); err != nil {
		tmpFile.Close()
		return err
	}

	// Write metadata blocks
	for i, b := range blocks {
		isLast := i == len(blocks)-1
		var header [4]byte
		if isLast {
			header[0] = b.blockType | 0x80
		} else {
			header[0] = b.blockType
		}
		length := len(b.data)
		header[1] = byte(length >> 16)
		header[2] = byte(length >> 8)
		header[3] = byte(length)

		if _, err := tmpFile.Write(header[:]); err != nil {
			tmpFile.Close()
			return err
		}
		if _, err := tmpFile.Write(b.data); err != nil {
			tmpFile.Close()
			return err
		}
	}

	// Copy remaining audio frames verbatim
	if _, err := io.Copy(tmpFile, f); err != nil {
		tmpFile.Close()
		return fmt.Errorf("tag flac: copy audio: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}
	f.Close()

	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("tag flac: rename: %w", err)
	}
	return nil
}

func buildVorbisComment(info TrackInfo) []byte {
	var comments []string

	add := func(key, val string) {
		if val != "" {
			comments = append(comments, key+"="+val)
		}
	}

	add("TITLE", info.Title)
	add("ARTIST", info.Artist)
	add("ALBUM", info.Album)
	add("ALBUMARTIST", info.AlbumArtist)
	add("GENRE", info.Genre)
	add("ISRC", info.ISRC)
	add("DATE", info.Year)
	if info.TrackNumber > 0 {
		add("TRACKNUMBER", strconv.Itoa(info.TrackNumber))
	}
	if info.TrackTotal > 0 {
		add("TRACKTOTAL", strconv.Itoa(info.TrackTotal))
	}
	if info.DiscNumber > 0 {
		add("DISCNUMBER", strconv.Itoa(info.DiscNumber))
	}
	if info.DiscTotal > 0 {
		add("DISCTOTAL", strconv.Itoa(info.DiscTotal))
	}
	add("LYRICS", info.Lyrics)

	var buf bytes.Buffer
	vendor := "MusicDownloader"
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(vendor)))
	buf.WriteString(vendor)

	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(comments)))
	for _, c := range comments {
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(c)))
		buf.WriteString(c)
	}

	return buf.Bytes()
}

func buildPictureBlock(cover []byte) []byte {
	var buf bytes.Buffer

	// Picture type 3 = Cover (front)
	_ = binary.Write(&buf, binary.BigEndian, uint32(3))

	// MIME type
	mime := "image/jpeg"
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(mime)))
	buf.WriteString(mime)

	// Description
	desc := "Cover"
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(desc)))
	buf.WriteString(desc)

	// Width, height, color depth (24 bits), indexed color count (0)
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	_ = binary.Write(&buf, binary.BigEndian, uint32(24))
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))

	// Picture data length and picture bytes
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(cover)))
	buf.Write(cover)

	return buf.Bytes()
}
