package deezer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// gwResponse is the envelope type for Deezer's private gw-light.php API.
type gwResponse[T any] struct {
	Results T   `json:"results"`
	Error   any `json:"error"`
}



type userDataResults struct {
	CheckForm string `json:"checkForm"`
	User      struct {
		Options struct {
			LicenseToken    string         `json:"license_token"`
			WebSoundQuality map[string]any `json:"web_sound_quality"`
		} `json:"OPTIONS"`
	} `json:"USER"`
}

type songDataResults struct {
	SNGID               string `json:"SNG_ID"`
	SNGTITLE            string `json:"SNG_TITLE"`
	ARTNAME             string `json:"ART_NAME"`
	ALBTITLE            string `json:"ALB_TITLE"`
	TRACKTOKEN          string `json:"TRACK_TOKEN"`
	MD5ORIGIN           string `json:"MD5_ORIGIN"`
	MEDIAVERSION        string `json:"MEDIA_VERSION"`
	DURATION            string `json:"DURATION"`
	ALBPICTURE          string `json:"ALB_PICTURE"`
	TRACKNUM            string `json:"TRACK_NUMBER"`
	DISKNUM             string `json:"DISK_NUMBER"`
	PHYSICALRELEASEDATE string `json:"PHYSICAL_RELEASE_DATE"`
}

type albumTracksResults struct {
	Data []songDataResults `json:"data"`
}

type trackURLResponse struct {
	Data []struct {
		Media []struct {
			Sources []struct {
				URL string `json:"url"`
			} `json:"sources"`
		} `json:"media"`
	} `json:"data"`
}



type publicTrack struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
	Artist   struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title       string `json:"title"`
		CoverMedium string `json:"cover_medium"`
	} `json:"album"`
}

type publicAlbum struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	CoverMedium string `json:"cover_medium"`
	ReleaseDate string `json:"release_date"`
	Artist      struct {
		Name string `json:"name"`
	} `json:"artist"`
}

type publicPlaylist struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Tracks struct {
		Data []publicTrack `json:"data"`
	} `json:"tracks"`
}

type publicArtist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type publicArtistAlbum struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	CoverMedium string `json:"cover_medium"`
	ReleaseDate string `json:"release_date"`
}



// GetTrack returns the full metadata for a track by its Deezer ID.
func (c *Client) GetTrack(ctx context.Context, id string) (Track, error) {
	body, _ := json.Marshal(map[string]string{"SNG_ID": id})
	req, err := c.buildPrivateRequest(ctx, http.MethodPost, "song.getData", body)
	if err != nil {
		return Track{}, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Track{}, fmt.Errorf("GetTrack(%s): %w", id, err)
	}
	defer resp.Body.Close()

	var result gwResponse[songDataResults]
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Track{}, fmt.Errorf("GetTrack(%s): decode: %w", id, err)
	}
	if hasGWError(result.Error) || result.Results.SNGID == "" {
		return Track{}, &ErrNotFound{Type: TypeTrack, ID: id}
	}
	return songDataToTrack(result.Results), nil
}

// GetAlbum returns an album with its full track list, including download tokens.
func (c *Client) GetAlbum(ctx context.Context, id string) (Album, error) {
	var pub publicAlbum
	if err := c.getPublic(ctx, "/album/"+id, &pub); err != nil {
		return Album{}, fmt.Errorf("GetAlbum(%s): %w", id, err)
	}
	if pub.ID == 0 {
		return Album{}, &ErrNotFound{Type: TypeAlbum, ID: id}
	}

	body, _ := json.Marshal(map[string]any{"ALB_ID": id, "lang": "en", "nb": 500})
	req, err := c.buildPrivateRequest(ctx, http.MethodPost, "song.getListByAlbum", body)
	if err != nil {
		return Album{}, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Album{}, fmt.Errorf("GetAlbum(%s) tracks: %w", id, err)
	}
	defer resp.Body.Close()

	var result gwResponse[albumTracksResults]
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Album{}, fmt.Errorf("GetAlbum(%s) tracks decode: %w", id, err)
	}

	tracks := make([]Track, len(result.Results.Data))
	for i, d := range result.Results.Data {
		tracks[i] = songDataToTrack(d)
	}

	return Album{
		ID:      fmt.Sprintf("%d", pub.ID),
		Title:   pub.Title,
		Artist:  pub.Artist.Name,
		Year:    yearFromDate(pub.ReleaseDate),
		CoverID: coverIDFromURL(pub.CoverMedium),
		Tracks:  tracks,
	}, nil
}

// GetPlaylist returns a playlist.
// Tracks from the public API lack download tokens; Download methods resolve them automatically.
func (c *Client) GetPlaylist(ctx context.Context, id string) (Playlist, error) {
	var pub publicPlaylist
	if err := c.getPublic(ctx, "/playlist/"+id, &pub); err != nil {
		return Playlist{}, fmt.Errorf("GetPlaylist(%s): %w", id, err)
	}
	if pub.ID == 0 {
		return Playlist{}, &ErrNotFound{Type: TypePlaylist, ID: id}
	}

	tracks := make([]Track, len(pub.Tracks.Data))
	for i, t := range pub.Tracks.Data {
		tracks[i] = Track{
			ID:     fmt.Sprintf("%d", t.ID),
			Title:  t.Title,
			Artist: t.Artist.Name,
			Album:  t.Album.Title,
		}
	}
	return Playlist{
		ID:     fmt.Sprintf("%d", pub.ID),
		Title:  pub.Title,
		Tracks: tracks,
	}, nil
}

// GetArtist returns an artist with a list of album stubs (albums have no tracks loaded).
// Use GetAlbum or DownloadArtist to work with full track data.
func (c *Client) GetArtist(ctx context.Context, id string) (Artist, error) {
	var pub publicArtist
	if err := c.getPublic(ctx, "/artist/"+id, &pub); err != nil {
		return Artist{}, fmt.Errorf("GetArtist(%s): %w", id, err)
	}
	if pub.ID == 0 {
		return Artist{}, &ErrNotFound{Type: TypeArtist, ID: id}
	}

	var albumList struct {
		Data []publicArtistAlbum `json:"data"`
	}
	if err := c.getPublic(ctx, fmt.Sprintf("/artist/%s/albums?limit=500", id), &albumList); err != nil {
		return Artist{}, fmt.Errorf("GetArtist(%s) albums: %w", id, err)
	}

	albums := make([]Album, len(albumList.Data))
	for i, a := range albumList.Data {
		albums[i] = Album{
			ID:      fmt.Sprintf("%d", a.ID),
			Title:   a.Title,
			Artist:  pub.Name,
			Year:    yearFromDate(a.ReleaseDate),
			CoverID: coverIDFromURL(a.CoverMedium),
		}
	}
	return Artist{
		ID:     fmt.Sprintf("%d", pub.ID),
		Name:   pub.Name,
		Albums: albums,
	}, nil
}

// Search returns tracks matching the query from the Deezer public API.
func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	var result struct {
		Data []publicTrack `json:"data"`
	}
	endpoint := fmt.Sprintf("/search?q=%s&type=track&limit=30", url.QueryEscape(query))
	if err := c.getPublic(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("Search(%q): %w", query, err)
	}

	tracks := make([]Track, len(result.Data))
	for i, t := range result.Data {
		tracks[i] = Track{
			ID:       fmt.Sprintf("%d", t.ID),
			Title:    t.Title,
			Artist:   t.Artist.Name,
			Album:    t.Album.Title,
			Duration: t.Duration,
		}
	}
	return tracks, nil
}

// SearchArtist searches for an artist by name and returns the top match with their albums.
func (c *Client) SearchArtist(ctx context.Context, query string) (Artist, error) {
	var result struct {
		Data []publicArtist `json:"data"`
	}
	endpoint := fmt.Sprintf("/search/artist?q=%s&limit=1", url.QueryEscape(query))
	if err := c.getPublic(ctx, endpoint, &result); err != nil {
		return Artist{}, fmt.Errorf("SearchArtist(%q): %w", query, err)
	}
	if len(result.Data) == 0 {
		return Artist{}, &ErrNotFound{Type: TypeArtist, ID: query}
	}
	return c.GetArtist(ctx, fmt.Sprintf("%d", result.Data[0].ID))
}

var deezerURLRegex = regexp.MustCompile(`deezer\.com/(?:[a-zA-Z]{2}(?:-[a-zA-Z]{2})?/)?(track|album|playlist|artist)/(\d+)`)

// ParseURL extracts the media type and numeric ID from a Deezer URL.
// It supports standard URLs, regional URLs (e.g. /us/album/123), and shortlinks (link.deezer.com).
func ParseURL(rawURL string) (MediaType, string, error) {
	m := deezerURLRegex.FindStringSubmatch(rawURL)
	if m != nil {
		return MediaType(m[1]), m[2], nil
	}

	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", userAgent)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				m = deezerURLRegex.FindStringSubmatch(resp.Request.URL.String())
				if m != nil {
					return MediaType(m[1]), m[2], nil
				}
			}
		}
	}

	return "", "", fmt.Errorf("not a recognized Deezer URL: %q", rawURL)
}



func (c *Client) getUserData() (licenseToken string, quality map[string]any, err error) {
	req, err := c.newRequest(http.MethodGet, privateAPIURL)
	if err != nil {
		return "", nil, err
	}
	q := req.URL.Query()
	q.Set("method", "deezer.getUserData")
	q.Set("api_version", "1.0")
	q.Set("api_token", "null")
	q.Set("input", "3")
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("getUserData: %w", err)
	}
	defer resp.Body.Close()

	var result gwResponse[userDataResults]
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("getUserData: decode: %w", err)
	}
	if hasGWError(result.Error) {
		return "", nil, fmt.Errorf("getUserData: API error: %v", result.Error)
	}

	c.apiToken = result.Results.CheckForm
	token := result.Results.User.Options.LicenseToken
	if token == "" {
		return "", nil, &ErrAuth{Reason: "license_token is empty — ARL may be invalid or expired"}
	}
	return token, result.Results.User.Options.WebSoundQuality, nil
}

func (c *Client) getTrackStreamURL(ctx context.Context, trackToken string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"license_token": c.licenseToken,
		"media": []map[string]any{{
			"type": "FULL",
			"formats": []map[string]string{
				{"cipher": "BF_CBC_STRIPE", "format": c.soundFormat},
			},
		}},
		"track_tokens": []string{trackToken},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaAPIURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("getTrackStreamURL: %w", err)
	}
	defer resp.Body.Close()

	var result trackURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("getTrackStreamURL: decode: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Media) == 0 || len(result.Data[0].Media[0].Sources) == 0 {
		return "", &ErrUnavailable{Reason: "empty response — license_token may be expired"}
	}
	return result.Data[0].Media[0].Sources[0].URL, nil
}

type lyricsResults struct {
	LyricsText string `json:"LYRICS_TEXT"`
	LyricsSync []struct {
		Timestamp string `json:"lrc_timestamp"`
		Line      string `json:"line"`
	} `json:"LYRICS_SYNC_JSON"`
}

type lrclibResponse struct {
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}

func (c *Client) getLyrics(ctx context.Context, track Track) (text string, lrc string, err error) {
	body, _ := json.Marshal(map[string]string{"SNG_ID": track.ID})
	req, err := c.buildPrivateRequest(ctx, http.MethodPost, "song.getLyrics", body)
	if err == nil {
		resp, err := c.http.Do(req)
		if err == nil {
			var result gwResponse[lyricsResults]
			if json.NewDecoder(resp.Body).Decode(&result) == nil {
				text = result.Results.LyricsText
				var sb strings.Builder
				for _, item := range result.Results.LyricsSync {
					if item.Timestamp != "" {
						sb.WriteString(item.Timestamp)
						sb.WriteString(item.Line)
						sb.WriteString("\n")
					}
				}
				lrc = sb.String()
			}
			resp.Body.Close()
		}
	}

	if text == "" && lrc == "" {
		text, lrc = c.fetchLRCLIB(track)
	}

	return text, lrc, nil
}

func (c *Client) fetchLRCLIB(track Track) (text string, lrc string) {
	u := fmt.Sprintf("https://lrclib.net/api/get?artist_name=%s&track_name=%s&album_name=%s&duration=%d",
		url.QueryEscape(track.Artist),
		url.QueryEscape(track.Title),
		url.QueryEscape(track.Album),
		track.Duration,
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var res lrclibResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", ""
	}
	return res.PlainLyrics, res.SyncedLyrics
}

func (c *Client) getCover(coverID string) ([]byte, error) {
	if coverID == "" {
		return nil, nil
	}
	resp, err := c.http.Get(fmt.Sprintf("%s/%s/1200x1200.jpg", coverBaseURL, coverID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}



func (c *Client) buildPrivateRequest(ctx context.Context, method, apiMethod string, body []byte) (*http.Request, error) {
	req, err := c.newRequest(method, privateAPIURL)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	q := req.URL.Query()
	q.Set("method", apiMethod)
	q.Set("api_version", "1.0")
	q.Set("api_token", c.apiToken)
	q.Set("input", "3")
	req.URL.RawQuery = q.Encode()

	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) getPublic(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, publicAPIURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func songDataToTrack(d songDataResults) Track {
	t := Track{
		ID:           d.SNGID,
		Title:        d.SNGTITLE,
		Artist:       d.ARTNAME,
		Album:        d.ALBTITLE,
		CoverID:      d.ALBPICTURE,
		trackToken:   d.TRACKTOKEN,
		md5Origin:    d.MD5ORIGIN,
		mediaVersion: d.MEDIAVERSION,
		Year:         yearFromDate(d.PHYSICALRELEASEDATE),
	}
	t.TrackNumber, _ = strconv.Atoi(d.TRACKNUM)
	t.DiscNumber, _ = strconv.Atoi(d.DISKNUM)
	t.Duration, _ = strconv.Atoi(d.DURATION)
	return t
}

func yearFromDate(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func coverIDFromURL(coverURL string) string {
	parts := strings.Split(coverURL, "/cover/")
	if len(parts) < 2 {
		return ""
	}
	return strings.SplitN(parts[1], "/", 2)[0]
}

func hasGWError(errAny any) bool {
	if errAny == nil {
		return false
	}
	switch v := errAny.(type) {
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return false
}
