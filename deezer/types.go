package deezer

// Quality represents the audio format requested from Deezer.
// Free accounts are capped at QualityMP3128 regardless of this setting.
type Quality string

const (
	QualityMP3128 Quality = "MP3_128"
	QualityMP3320 Quality = "MP3_320"
)

// MediaType identifies the kind of Deezer resource.
type MediaType string

const (
	TypeTrack    MediaType = "track"
	TypeAlbum    MediaType = "album"
	TypePlaylist MediaType = "playlist"
	TypeArtist   MediaType = "artist"
)

// Track holds the metadata and internal state needed to download a Deezer track.
type Track struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	TrackNumber int
	DiscNumber  int
	Year        string
	Duration    int // seconds
	CoverID     string

	// Internal fields populated by the private Deezer API.
	// Not exported: callers use Download methods, not raw tokens.
	trackToken   string
	md5Origin    string
	mediaVersion string
}

// Album represents a Deezer album with its full track list.
type Album struct {
	ID      string
	Title   string
	Artist  string
	Year    string
	CoverID string
	Tracks  []Track
}

// Playlist represents a Deezer playlist.
type Playlist struct {
	ID     string
	Title  string
	Tracks []Track
}

// Artist contains basic metadata and a list of album stubs.
// Albums returned by GetArtist have no tracks populated;
// use GetAlbum or DownloadArtist to work with full track data.
type Artist struct {
	ID     string
	Name   string
	Albums []Album
}
