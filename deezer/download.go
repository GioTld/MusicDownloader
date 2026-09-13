package deezer

import (
	"context"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/blowfish"

	"github.com/GioTld/MusicDownloader/tag"
)

// Download auto-detects the resource type from rawInput (URL or numeric ID) and downloads it to dir.
// If rawInput is not a recognized Deezer URL or ID, it searches for an artist matching rawInput
// and downloads their complete discography.
func (c *Client) Download(ctx context.Context, rawInput, dir string) ([]string, error) {
	kind, id, err := ParseURL(rawInput)
	if err == nil {
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

	artist, searchErr := c.SearchArtist(ctx, rawInput)
	if searchErr != nil {
		return nil, fmt.Errorf("invalid Deezer URL or artist search %q: %w", rawInput, searchErr)
	}
	return c.DownloadArtist(ctx, artist.ID, dir)
}

type albumContext struct {
	AlbumIndex  int
	TotalAlbums int
	AlbumTitle  string
	AlbumYear   string
}

// DownloadTrack downloads a single track by ID to dir.
func (c *Client) DownloadTrack(ctx context.Context, id, dir string) (string, error) {
	track, err := c.GetTrack(ctx, id)
	if err != nil {
		return "", err
	}
	if c.opts.OnTargetResolved != nil {
		c.opts.OnTargetResolved(TargetDetails{
			Kind:        TypeTrack,
			Name:        fmt.Sprintf("%s - %s", track.Artist, track.Title),
			TotalAlbums: 1,
			TotalTracks: 1,
		})
	}
	actx := albumContext{AlbumIndex: 1, TotalAlbums: 1, AlbumTitle: track.Album, AlbumYear: track.Year}
	paths, err := c.downloadTracks(ctx, []Track{track}, actx, func(t Track) (string, string) {
		return dir, standaloneFilename(t)
	})
	if len(paths) > 0 {
		return paths[0], err
	}
	return "", err
}

// DownloadAlbum downloads all tracks of an album by ID.
func (c *Client) DownloadAlbum(ctx context.Context, id, dir string) ([]string, error) {
	album, err := c.GetAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.opts.OnTargetResolved != nil {
		c.opts.OnTargetResolved(TargetDetails{
			Kind:        TypeAlbum,
			Name:        album.Title,
			TotalAlbums: 1,
			TotalTracks: len(album.Tracks),
		})
	}
	destDir := filepath.Join(dir, sanitize(albumDirName(album)))
	actx := albumContext{AlbumIndex: 1, TotalAlbums: 1, AlbumTitle: album.Title, AlbumYear: album.Year}
	if c.opts.OnAlbumStart != nil {
		c.opts.OnAlbumStart(1, 1, album)
	}
	return c.downloadTracks(ctx, album.Tracks, actx, func(t Track) (string, string) {
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
	if c.opts.OnTargetResolved != nil {
		c.opts.OnTargetResolved(TargetDetails{
			Kind:        TypePlaylist,
			Name:        playlist.Title,
			TotalAlbums: 1,
			TotalTracks: len(tracks),
		})
	}
	destDir := filepath.Join(dir, sanitize(playlist.Title))
	actx := albumContext{AlbumIndex: 1, TotalAlbums: 1, AlbumTitle: playlist.Title}
	if c.opts.OnAlbumStart != nil {
		c.opts.OnAlbumStart(1, 1, Album{Title: playlist.Title, Tracks: tracks})
	}
	return c.downloadTracks(ctx, tracks, actx, func(t Track) (string, string) {
		return destDir, standaloneFilename(t)
	})
}

// DownloadArtist downloads the complete discography of an artist by ID.
func (c *Client) DownloadArtist(ctx context.Context, id, dir string) ([]string, error) {
	artist, err := c.GetArtist(ctx, id)
	if err != nil {
		return nil, err
	}

	var loadedAlbums []Album
	totalTracks := 0
	for _, stub := range artist.Albums {
		if ctx.Err() != nil {
			break
		}
		album, err := c.GetAlbum(ctx, stub.ID)
		if err != nil {
			continue
		}
		loadedAlbums = append(loadedAlbums, album)
		totalTracks += len(album.Tracks)
	}

	if c.opts.OnTargetResolved != nil {
		c.opts.OnTargetResolved(TargetDetails{
			Kind:        TypeArtist,
			Name:        artist.Name,
			TotalAlbums: len(loadedAlbums),
			TotalTracks: totalTracks,
		})
	}

	artistDir := filepath.Join(dir, sanitize(artist.Name))
	var allPaths []string
	var allErrs []string

	for i, album := range loadedAlbums {
		if ctx.Err() != nil {
			break
		}
		if c.opts.OnAlbumStart != nil {
			c.opts.OnAlbumStart(i+1, len(loadedAlbums), album)
		}
		albumDir := filepath.Join(artistDir, sanitize(albumDirName(album)))
		actx := albumContext{
			AlbumIndex:  i + 1,
			TotalAlbums: len(loadedAlbums),
			AlbumTitle:  album.Title,
			AlbumYear:   album.Year,
		}
		paths, err := c.downloadTracks(ctx, album.Tracks, actx, func(t Track) (string, string) {
			return albumDir, albumFilename(t)
		})
		allPaths = append(allPaths, paths...)
		if err != nil {
			allErrs = append(allErrs, fmt.Sprintf("%s: %v", album.Title, err))
		}
	}

	if len(allErrs) > 0 {
		return allPaths, fmt.Errorf("some albums had errors:\n%s", strings.Join(allErrs, "\n"))
	}
	return allPaths, nil
}

// downloadTracks downloads a slice of tracks concurrently, bounded by c.opts.Concurrency.
// pathFn returns the destination directory and filename for each individual track.
func (c *Client) downloadTracks(ctx context.Context, tracks []Track, actx albumContext, pathFn func(Track) (dir, filename string)) ([]string, error) {
	if len(tracks) == 0 {
		return nil, nil
	}

	type result struct {
		track  Track
		path   string
		status TrackStatus
		err    error
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
				path, status, err := c.downloadOne(ctx, track, d, f)
				out <- result{track: track, path: path, status: status, err: err}
			}(t)
		}
	}

	var paths []string
	var errs []string
	done := 0

	for range dispatched {
		r := <-out
		done++
		tr := TrackResult{
			Track:       r.track,
			Status:      r.status,
			Err:         r.err,
			AlbumIndex:  actx.AlbumIndex,
			TotalAlbums: actx.TotalAlbums,
			AlbumTitle:  actx.AlbumTitle,
			AlbumYear:   actx.AlbumYear,
			TrackIndex:  done,
			TotalTracks: len(tracks),
			FilePath:    r.path,
		}

		if c.opts.OnTrackComplete != nil {
			c.opts.OnTrackComplete(tr)
		}
		if c.opts.OnProgress != nil && r.status == StatusDownloaded {
			c.opts.OnProgress(done, len(tracks), r.track)
		}

		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s - %s: %v", r.track.Artist, r.track.Title, r.err))
			continue
		}
		paths = append(paths, r.path)
	}

	if cancelled {
		return paths, ctx.Err()
	}
	if len(errs) > 0 {
		return paths, fmt.Errorf("%d/%d tracks failed:\n%s", len(errs), len(tracks), strings.Join(errs, "\n"))
	}
	return paths, nil
}

func (c *Client) downloadOne(ctx context.Context, track Track, dir, filename string) (string, TrackStatus, error) {
	if ctx.Err() != nil {
		return "", StatusFailed, ctx.Err()
	}
	if track.trackToken == "" {
		err := &ErrUnavailable{TrackID: track.ID, Reason: "no track token"}
		return "", StatusFailed, err
	}

	destPath := filepath.Join(dir, filename)

	if c.opts.SkipExisting {
		if _, err := os.Stat(destPath); err == nil {
			return destPath, StatusSkipped, nil
		}
	}

	streamURL, err := c.getTrackStreamURL(ctx, track.trackToken)
	if err != nil {
		return "", StatusFailed, fmt.Errorf("track %s: stream URL: %w", track.ID, err)
	}

	var encrypted []byte
	if err := withRetry(func() error {
		var e error
		encrypted, e = c.fetchStream(ctx, streamURL)
		return e
	}); err != nil {
		return "", StatusFailed, fmt.Errorf("track %s: fetch: %w", track.ID, err)
	}

	decrypted, err := decrypt(encrypted, deriveKey(track.ID))
	if err != nil {
		return "", StatusFailed, fmt.Errorf("track %s: decrypt: %w", track.ID, err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", StatusFailed, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(destPath, decrypted, 0644); err != nil {
		return "", StatusFailed, err
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

	return destPath, StatusDownloaded, nil
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
	var hx [32]byte
	hex.Encode(hx[:], h[:])
	key := make([]byte, 16)
	for i := range 16 {
		key[i] = hx[i] ^ hx[i+16] ^ blowfishSecret[i]
	}
	return key
}

// decrypt decrypts a BF_CBC_STRIPE stream: 2048-byte chunks where every third
// one (0, 3, 6, …) is Blowfish-CBC encrypted; the rest are plaintext.
// A partial final chunk is never encrypted.
// Decryption is performed in-place to avoid heap allocations.
func decrypt(data, key []byte) ([]byte, error) {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(data); i += chunkSize {
		if (i/chunkSize)%encryptEvery == 0 && i+chunkSize <= len(data) {
			chunk := data[i : i+chunkSize]
			cipher.NewCBCDecrypter(block, blowfishIV).CryptBlocks(chunk, chunk)
		}
	}
	return data, nil
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

func isInvalidFilenameChar(r rune) rune {
	if r < 0x20 {
		return '_'
	}
	switch r {
	case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
		return '_'
	default:
		return r
	}
}

func sanitize(name string) string {
	name = strings.Map(isInvalidFilenameChar, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "unknown"
	}
	return name
}
