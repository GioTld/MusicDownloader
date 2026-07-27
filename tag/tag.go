package tag

import (
	"fmt"

	id3 "github.com/bogem/id3v2/v2"
)

// TrackInfo holds the metadata to write into an MP3 file.
type TrackInfo struct {
	Title       string
	Artist      string
	Album       string
	TrackNumber int
	DiscNumber  int
	Year        string
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

	if info.TrackNumber > 0 {
		f.AddTextFrame("TRCK", id3.EncodingUTF8, fmt.Sprintf("%d", info.TrackNumber))
	}
	if info.DiscNumber > 1 {
		f.AddTextFrame("TPOS", id3.EncodingUTF8, fmt.Sprintf("%d", info.DiscNumber))
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
