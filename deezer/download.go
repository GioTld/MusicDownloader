package deezer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
		return dir, standaloneFilenameWithExt(t, c.ext())
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
		return destDir, albumFilenameWithExt(t, c.ext())
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
		return destDir, standaloneFilenameWithExt(t, c.ext())
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
			return albumDir, albumFilenameWithExt(t, c.ext())
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
		if fi, err := os.Stat(destPath); err == nil {
			if !c.opts.SyncMode {
				if fi.Size() > 0 {
					return destPath, StatusSkipped, nil
				}
			} else {
				if validateTrackFile(destPath, fi.Size()) {
					return destPath, StatusSkipped, nil
				}
			}
			// Incomplete or corrupt file: remove and re-download
			_ = os.Remove(destPath)
		}
	}

	streamURL, err := c.getTrackStreamURL(ctx, track.trackToken)
	if err != nil {
		return "", StatusFailed, fmt.Errorf("track %s: stream URL: %w", track.ID, err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", StatusFailed, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Prefetch lyrics and cover art in the background while streaming audio.
	type metaResult struct {
		lyricsText string
		syncedLRC  string
		cover      []byte
	}
	metaCh := make(chan metaResult, 1)
	go func() {
		var mr metaResult
		if c.opts.EmbedLyrics || c.opts.SaveLRC {
			mr.lyricsText, mr.syncedLRC, _ = c.getLyrics(ctx, track)
		}
		mr.cover, _ = c.getCover(track.CoverID)
		metaCh <- mr
	}()

	key := deriveKey(track.ID)
	if err := withRetry(func() error {
		f, e := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if e != nil {
			return e
		}
		bw := bufio.NewWriterSize(f, 64*1024)
		sErr := c.downloadStream(ctx, streamURL, bw, key)
		fErr := bw.Flush()
		cErr := f.Close()
		if sErr != nil {
			_ = os.Remove(destPath)
			return sErr
		}
		if fErr != nil {
			_ = os.Remove(destPath)
			return fErr
		}
		return cErr
	}); err != nil {
		<-metaCh // drain background goroutine
		return "", StatusFailed, fmt.Errorf("track %s: download: %w", track.ID, err)
	}

	meta := <-metaCh

	if c.opts.SaveLRC && meta.syncedLRC != "" {
		ext := filepath.Ext(filename)
		lrcPath := filepath.Join(dir, strings.TrimSuffix(filename, ext)+".lrc")
		_ = os.WriteFile(lrcPath, []byte(meta.syncedLRC), 0644)
	}

	var lyricsTag string
	if c.opts.EmbedLyrics {
		lyricsTag = meta.lyricsText
	}

	_ = tag.TagFile(destPath, tag.TrackInfo{
		Title:       track.Title,
		Artist:      track.Artist,
		AlbumArtist: track.AlbumArtist,
		Album:       track.Album,
		Genre:       track.Genre,
		ISRC:        track.ISRC,
		TrackNumber: track.TrackNumber,
		TrackTotal:  track.TrackTotal,
		DiscNumber:  track.DiscNumber,
		DiscTotal:   track.DiscTotal,
		Year:        track.Year,
		Lyrics:      lyricsTag,
	}, meta.cover)

	return destPath, StatusDownloaded, nil
}

func (c *Client) downloadStream(ctx context.Context, streamURL string, w io.Writer, key []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ErrUnavailable{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return decryptStream(resp.Body, w, key)
}

// decryptStream streams and decrypts BF_CBC_STRIPE data from r directly to w
// using a single 2048-byte buffer without allocating memory for the full file.
func decryptStream(r io.Reader, w io.Writer, key []byte) error {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return err
	}
	buf := make([]byte, chunkSize)
	chunkIdx := 0

	for {
		n, err := io.ReadFull(r, buf)
		if n == chunkSize {
			if chunkIdx%encryptEvery == 0 {
				cipher.NewCBCDecrypter(block, blowfishIV).CryptBlocks(buf, buf)
			}
			if _, wErr := w.Write(buf); wErr != nil {
				return wErr
			}
			chunkIdx++
		} else if n > 0 {
			// Final partial chunk is never encrypted
			if _, wErr := w.Write(buf[:n]); wErr != nil {
				return wErr
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
	}
	return nil
}

// resolveTokens fetches full track data for tracks that only have an ID concurrently,
// preserving the original track order.
func (c *Client) resolveTokens(ctx context.Context, tracks []Track) ([]Track, error) {
	if len(tracks) == 0 {
		return nil, nil
	}

	resolved := make([]Track, len(tracks))
	copy(resolved, tracks)

	type task struct {
		idx int
		id  string
	}

	var tasks []task
	for i, t := range tracks {
		if t.trackToken == "" {
			tasks = append(tasks, task{idx: i, id: t.ID})
		}
	}

	if len(tasks) == 0 {
		return resolved, nil
	}

	concurrency := min(c.opts.Concurrency*2, len(tasks))
	if concurrency <= 0 {
		concurrency = 3
	}

	taskCh := make(chan task, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	type result struct {
		idx  int
		info Track
		err  error
	}
	resCh := make(chan result, len(tasks))

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskCh {
				if ctx.Err() != nil {
					resCh <- result{idx: t.idx, err: ctx.Err()}
					return
				}
				info, err := c.GetTrack(ctx, t.id)
				resCh <- result{idx: t.idx, info: info, err: err}
			}
		}()
	}

	wg.Wait()
	close(resCh)

	for r := range resCh {
		if r.err != nil {
			return nil, fmt.Errorf("resolving track %d: %w", r.idx, r.err)
		}
		resolved[r.idx] = r.info
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

func (c *Client) ext() string {
	if c.soundFormat == "FLAC" {
		return ".flac"
	}
	return ".mp3"
}

func standaloneFilename(t Track) string {
	return standaloneFilenameWithExt(t, ".mp3")
}

func standaloneFilenameWithExt(t Track, ext string) string {
	return sanitize(fmt.Sprintf("%s - %s%s", t.Artist, t.Title, ext))
}

func albumFilename(t Track) string {
	return albumFilenameWithExt(t, ".mp3")
}

func albumFilenameWithExt(t Track, ext string) string {
	if t.TrackNumber > 0 {
		return sanitize(fmt.Sprintf("%02d - %s%s", t.TrackNumber, t.Title, ext))
	}
	return sanitize(t.Title + ext)
}

// validateTrackFile verifies that a file has at least 100 KB and valid magic header bytes.
func validateTrackFile(filePath string, size int64) bool {
	if size < 100*1024 {
		return false
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	var hdr [4]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return false
	}

	lower := strings.ToLower(filePath)
	if strings.HasSuffix(lower, ".flac") {
		return bytes.Equal(hdr[:], []byte("fLaC"))
	}
	// MP3: check ID3v2 ("ID3") or MPEG frame sync (11 bits all 1s: 0xFF 0xEx)
	if string(hdr[:3]) == "ID3" {
		return true
	}
	if hdr[0] == 0xFF && (hdr[1]&0xE0) == 0xE0 {
		return true
	}
	return false
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
