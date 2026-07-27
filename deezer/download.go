package deezer

import (
	"context"
	"crypto/cipher"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/blowfish"

	"github.com/GioPelao2/MusicDownloader/tag"
)

// Download auto-detects the resource type from rawURL and downloads it to dir.
//
//	track    → dir/Artist - Title.mp3
//	album    → dir/(Year) Album/01 - Title.mp3
//	playlist → dir/Playlist Name/Artist - Title.mp3
//	artist   → dir/Artist Name/(Year) Album/01 - Title.mp3
func (c *Client) Download(ctx context.Context, rawURL, dir string) ([]string, error) {
	kind, id, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	switch kind {
	case TypeTrack:
		path, err := c.DownloadTrack(ctx, id, dir)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	case TypeAlbum:
		return c.DownloadAlbum(ctx, id, dir)
	case TypePlaylist:
		return c.DownloadPlaylist(ctx, id, dir)
	case TypeArtist:
		return c.DownloadArtist(ctx, id, dir)
	default:
		return nil, fmt.Errorf("unsupported type: %s", kind)
	}
}

// DownloadTrack downloads a single track by ID to dir.
func (c *Client) DownloadTrack(ctx context.Context, id, dir string) (string, error) {
	track, err := c.GetTrack(ctx, id)
	if err != nil {
		return "", err
	}
	return c.downloadOne(ctx, track, dir, standaloneFilename(track))
}

// DownloadAlbum downloads all tracks of an album by ID.
func (c *Client) DownloadAlbum(ctx context.Context, id, dir string) ([]string, error) {
	album, err := c.GetAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	destDir := filepath.Join(dir, sanitize(albumDirName(album)))
	return c.downloadTracks(ctx, album.Tracks, func(t Track) (string, string) {
		return destDir, albumFilename(t)
	})
}

// DownloadPlaylist downloads all tracks of a playlist by ID.
func (c *Client) DownloadPlaylist(ctx context.Context, id, dir string) ([]string, error) {
	playlist, err := c.GetPlaylist(ctx, id)
	if err != nil {
		return nil, err
	}
	// Playlist tracks from the public API lack download tokens; resolve them first.
	tracks, err := c.resolveTokens(ctx, playlist.Tracks)
	if err != nil {
		return nil, err
	}
	destDir := filepath.Join(dir, sanitize(playlist.Title))
	return c.downloadTracks(ctx, tracks, func(t Track) (string, string) {
		return destDir, standaloneFilename(t)
	})
}

// DownloadArtist downloads the complete discography of an artist by ID.
func (c *Client) DownloadArtist(ctx context.Context, id, dir string) ([]string, error) {
	artist, err := c.GetArtist(ctx, id)
	if err != nil {
		return nil, err
	}

	artistDir := filepath.Join(dir, sanitize(artist.Name))
	var allPaths []string

	for _, stub := range artist.Albums {
		if ctx.Err() != nil {
			break
		}
		album, err := c.GetAlbum(ctx, stub.ID)
		if err != nil {
			fmt.Printf("warning: skipping album %q: %v\n", stub.Title, err)
			continue
		}
		albumDir := filepath.Join(artistDir, sanitize(albumDirName(album)))
		paths, err := c.downloadTracks(ctx, album.Tracks, func(t Track) (string, string) {
			return albumDir, albumFilename(t)
		})
		allPaths = append(allPaths, paths...)
		if err != nil {
			fmt.Printf("warning: some tracks in %q failed: %v\n", album.Title, err)
		}
	}
	return allPaths, nil
}

// downloadTracks downloads a slice of tracks concurrently, bounded by c.opts.Concurrency.
// pathFn returns the destination directory and filename for each individual track.
func (c *Client) downloadTracks(ctx context.Context, tracks []Track, pathFn func(Track) (dir, filename string)) ([]string, error) {
	if len(tracks) == 0 {
		return nil, nil
	}

	type result struct {
		track Track
		path  string
		err   error
	}

	out := make(chan result, len(tracks))
	sem := make(chan struct{}, c.opts.Concurrency)

	dispatched := 0
	cancelled := false
	for _, t := range tracks {
		if cancelled {
			break
		}
		select {
		case <-ctx.Done():
			cancelled = true
		case sem <- struct{}{}:
			dispatched++
			go func(track Track) {
				defer func() { <-sem }()
				d, f := pathFn(track)
				path, err := c.downloadOne(ctx, track, d, f)
				out <- result{track: track, path: path, err: err}
			}(t)
		}
	}

	var paths []string
	var errs []string
	done := 0

	for range dispatched {
		r := <-out
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s - %s: %v", r.track.Artist, r.track.Title, r.err))
			continue
		}
		paths = append(paths, r.path)
		done++
		if c.opts.OnProgress != nil {
			c.opts.OnProgress(done, len(tracks), r.track)
		}
	}

	if cancelled {
		return paths, ctx.Err()
	}
	if len(errs) > 0 {
		return paths, fmt.Errorf("%d/%d tracks failed:\n%s", len(errs), len(tracks), strings.Join(errs, "\n"))
	}
	return paths, nil
}


func (c *Client) downloadOne(ctx context.Context, track Track, dir, filename string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if track.trackToken == "" {
		return "", &ErrUnavailable{TrackID: track.ID, Reason: "no track token"}
	}

	destPath := filepath.Join(dir, filename)

	if c.opts.SkipExisting {
		if _, err := os.Stat(destPath); err == nil {
			return destPath, nil
		}
	}

	streamURL, err := c.getTrackStreamURL(ctx, track.trackToken)
	if err != nil {
		return "", fmt.Errorf("track %s: stream URL: %w", track.ID, err)
	}

	var encrypted []byte
	if err := withRetry(func() error {
		var e error
		encrypted, e = c.fetchStream(ctx, streamURL)
		return e
	}); err != nil {
		return "", fmt.Errorf("track %s: fetch: %w", track.ID, err)
	}

	decrypted, err := decrypt(encrypted, deriveKey(track.ID))
	if err != nil {
		return "", fmt.Errorf("track %s: decrypt: %w", track.ID, err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(destPath, decrypted, 0644); err != nil {
		return "", err
	}

	var lyricsText, syncedLRC string
	if c.opts.EmbedLyrics || c.opts.SaveLRC {
		lyricsText, syncedLRC, _ = c.getLyrics(ctx, track)
	}

	if c.opts.SaveLRC && syncedLRC != "" {
		lrcPath := filepath.Join(dir, strings.TrimSuffix(filename, ".mp3")+".lrc")
		_ = os.WriteFile(lrcPath, []byte(syncedLRC), 0644)
	}

	cover, _ := c.getCover(track.CoverID)
	var lyricsTag string
	if c.opts.EmbedLyrics {
		lyricsTag = lyricsText
	}

	_ = tag.TagMP3(destPath, tag.TrackInfo{
		Title:       track.Title,
		Artist:      track.Artist,
		Album:       track.Album,
		TrackNumber: track.TrackNumber,
		DiscNumber:  track.DiscNumber,
		Year:        track.Year,
		Lyrics:      lyricsTag,
	}, cover)

	return destPath, nil
}

func (c *Client) fetchStream(ctx context.Context, streamURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ErrUnavailable{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return io.ReadAll(resp.Body)
}

// resolveTokens fetches full track data for tracks that only have an ID.
// Playlist responses from the public API return tracks without download tokens.
func (c *Client) resolveTokens(ctx context.Context, tracks []Track) ([]Track, error) {
	resolved := make([]Track, 0, len(tracks))
	for _, t := range tracks {
		if t.trackToken != "" {
			resolved = append(resolved, t)
			continue
		}
		info, err := c.GetTrack(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("resolving track %s: %w", t.ID, err)
		}
		resolved = append(resolved, info)
	}
	return resolved, nil
}

// deriveKey computes the 16-byte Blowfish key for a track.
// It XORs each byte of the two MD5 hex halves of the track ID
// against a secret hardcoded in Deezer's web player JavaScript.
func deriveKey(trackID string) []byte {
	h := md5.Sum([]byte(trackID))
	hx := fmt.Sprintf("%x", h)
	key := make([]byte, 16)
	for i := range 16 {
		key[i] = hx[i] ^ hx[i+16] ^ blowfishSecret[i]
	}
	return key
}

// decrypt decrypts a BF_CBC_STRIPE stream: 2048-byte chunks where every third
// one (0, 3, 6, …) is Blowfish-CBC encrypted; the rest are plaintext.
// A partial final chunk is never encrypted.
func decrypt(data, key []byte) ([]byte, error) {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		chunk := data[i:end]
		if (i/chunkSize)%encryptEvery == 0 && len(chunk) == chunkSize {
			dec := make([]byte, chunkSize)
			cipher.NewCBCDecrypter(block, blowfishIV).CryptBlocks(dec, chunk)
			out = append(out, dec...)
		} else {
			out = append(out, chunk...)
		}
	}
	return out, nil
}

func standaloneFilename(t Track) string {
	return sanitize(fmt.Sprintf("%s - %s.mp3", t.Artist, t.Title))
}

func albumFilename(t Track) string {
	if t.TrackNumber > 0 {
		return sanitize(fmt.Sprintf("%02d - %s.mp3", t.TrackNumber, t.Title))
	}
	return sanitize(t.Title + ".mp3")
}

func albumDirName(a Album) string {
	if a.Year != "" {
		return fmt.Sprintf("(%s) %s", a.Year, a.Title)
	}
	return a.Title
}

func sanitize(name string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	name = re.ReplaceAllString(name, "_")
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "unknown"
	}
	return name
}
