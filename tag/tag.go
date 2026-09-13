package tag

import (
	"fmt"
	"strings"

	id3 "github.com/bogem/id3v2/v2"
)

// TrackInfo holds the metadata to write into an MP3 or FLAC file.
type TrackInfo struct {
	Title       string
	Artist      string
	AlbumArtist string // TPE2 / ALBUMARTIST
	Album       string
	Genre       string // TCON / GENRE
	ISRC        string // TSRC / ISRC
	TrackNumber int
	TrackTotal  int // Total tracks on the disc/album
	DiscNumber  int
	DiscTotal   int // Total discs in the album
	Year        string
	Lyrics      string
}

// TagFile writes metadata tags to an audio file (MP3 or FLAC).
// It dispatches to TagFLAC for .flac files and TagMP3 for .mp3 files.
func TagFile(filePath string, info TrackInfo, cover []byte) error {
	if strings.HasSuffix(strings.ToLower(filePath), ".flac") {
		return TagFLAC(filePath, info, cover)
	}
	return TagMP3(filePath, info, cover)
}

// TagMP3 writes ID3v2 tags to the MP3 file at filePath.
// cover should be JPEG bytes; pass nil to skip the cover art frame.
// Tagging errors are returned but the file remains usable.
func TagMP3(filePath string, info TrackInfo, cover []byte) error {
	f, err := id3.Open(filePath, id3.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("tag: open %s: %w", filePath, err)
	}
	defer f.Close()

	f.SetVersion(3)
	f.SetTitle(info.Title)
	f.SetArtist(info.Artist)
	f.SetAlbum(info.Album)
	f.SetYear(info.Year)

	if info.AlbumArtist != "" {
		f.AddTextFrame("TPE2", id3.EncodingUTF8, info.AlbumArtist)
	}
	if info.Genre != "" {
		f.AddTextFrame("TCON", id3.EncodingUTF8, info.Genre)
	}
	if info.ISRC != "" {
		f.AddTextFrame("TSRC", id3.EncodingUTF8, info.ISRC)
	}

	if info.TrackNumber > 0 {
		if info.TrackTotal > 0 {
			f.AddTextFrame("TRCK", id3.EncodingUTF8, fmt.Sprintf("%d/%d", info.TrackNumber, info.TrackTotal))
		} else {
			f.AddTextFrame("TRCK", id3.EncodingUTF8, fmt.Sprintf("%d", info.TrackNumber))
		}
	}
	if info.DiscNumber > 1 || info.DiscTotal > 1 {
		discNum := max(1, info.DiscNumber)
		if info.DiscTotal > 0 {
			f.AddTextFrame("TPOS", id3.EncodingUTF8, fmt.Sprintf("%d/%d", discNum, info.DiscTotal))
		} else {
			f.AddTextFrame("TPOS", id3.EncodingUTF8, fmt.Sprintf("%d", discNum))
		}
	}
	if info.Lyrics != "" {
		f.AddUnsynchronisedLyricsFrame(id3.UnsynchronisedLyricsFrame{
			Encoding:          id3.EncodingUTF8,
			Language:          "eng",
			ContentDescriptor: "Lyrics",
			Lyrics:            info.Lyrics,
		})
	}
	if len(cover) > 0 {
		f.AddAttachedPicture(id3.PictureFrame{
			Encoding:    id3.EncodingUTF8,
			MimeType:    "image/jpeg",
			PictureType: id3.PTFrontCover,
			Description: "Cover",
			Picture:     cover,
		})
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("tag: save %s: %w", filePath, err)
	}
	return nil
}
