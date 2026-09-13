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

// TrackStatus indicates the outcome of downloading an individual track.
type TrackStatus int

const (
	StatusDownloaded TrackStatus = iota
	StatusSkipped
	StatusFailed
)

// TargetDetails describes the resolved media resource before downloading starts.
type TargetDetails struct {
	Kind        MediaType
	Name        string
	TotalAlbums int
	TotalTracks int
}

// TrackResult holds progress details and status for a completed or skipped track download.
type TrackResult struct {
	Track       Track
	Status      TrackStatus
	Err         error
	AlbumIndex  int // 1-indexed, 0 if not applicable
	TotalAlbums int // 0 if not applicable
	AlbumTitle  string
	AlbumYear   string
	TrackIndex  int // 1-indexed within the current album/batch
	TotalTracks int // Total tracks in the current album/batch
	FilePath    string
}

