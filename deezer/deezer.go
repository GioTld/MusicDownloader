// Package deezer provides a client for downloading music from Deezer.
//
// Authentication requires a valid ARL cookie from a logged-in Deezer session.
// Obtain it via browser DevTools → Application → Cookies → deezer.com → arl.
// ARL cookies expire roughly every 3 months.
//
// Basic usage:
//
//	client, err := deezer.New(deezer.Options{ARL: "your_arl_cookie"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	files, err := client.Download(ctx, "https://www.deezer.com/album/302127", "./music")
package deezer

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

// ProgressFunc is called after each track download completes.
// done is the number of tracks finished so far; total is the count for the current batch.
type ProgressFunc func(done, total int, track Track)

// Options configures a Client.
type Options struct {
	// ARL is the Deezer authentication cookie. Required.
	ARL string

	// Quality is the preferred audio format.
	// Defaults to QualityMP3320; free accounts are capped at QualityMP3128.
	Quality Quality

	// Concurrency is the number of tracks downloaded simultaneously. Defaults to 3.
	Concurrency int

	// SkipExisting skips a track if its output file already exists on disk.
	SkipExisting bool

	// EmbedLyrics embeds plain-text lyrics into the MP3 ID3v2 tag if available.
	EmbedLyrics bool

	// SaveLRC saves a synced .lrc lyrics file alongside the MP3 if available.
	SaveLRC bool

	// Proxy is an optional HTTP proxy URL (e.g., "http://127.0.0.1:8080").
	Proxy string

	// Transport is an optional HTTP RoundTripper (e.g., for testing with mocks).
	Transport http.RoundTripper

	// OnProgress is called after each track completes. May be nil.
	OnProgress ProgressFunc

	// OnTargetResolved is called when the Deezer resource (artist, album, playlist, track) is identified.
	OnTargetResolved func(TargetDetails)

	// OnAlbumStart is called before downloading an album's tracks.
	OnAlbumStart func(albumIndex, totalAlbums int, album Album)

	// OnTrackComplete is called whenever a track download succeeds, is skipped, or fails.
	OnTrackComplete func(TrackResult)
}

// Client is an authenticated Deezer client. Create one with New.
type Client struct {
	http         *http.Client
	apiToken     string
	licenseToken string
	soundFormat  string
	opts         Options
	coverCache   sync.Map
}

// New creates an authenticated Client.
// It connects to Deezer immediately to validate the ARL and fetch API tokens.
func New(opts Options) (*Client, error) {
	if opts.ARL == "" {
		return nil, &ErrAuth{Reason: "ARL is required"}
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 8
	}
	if opts.Quality == "" {
		opts.Quality = QualityMP3320
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	deezerURL, _ := url.Parse("https://www.deezer.com")
	jar.SetCookies(deezerURL, []*http.Cookie{
		{Name: "arl", Value: opts.ARL},
		{Name: "comeback", Value: "1"},
	})

	var rt http.RoundTripper
	if opts.Transport != nil {
		rt = opts.Transport
	} else {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.MaxIdleConns = 100
		tr.MaxIdleConnsPerHost = max(10, opts.Concurrency*4)
		tr.IdleConnTimeout = 90 * time.Second
		tr.DisableCompression = true
		if opts.Proxy != "" {
			proxyURL, err := url.Parse(opts.Proxy)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy URL: %w", err)
			}
			tr.Proxy = http.ProxyURL(proxyURL)
		}
		rt = tr
	}

	c := &Client{
		http: &http.Client{
			Jar:       jar,
			Transport: rt,
			Timeout:   60 * time.Second,
		},
		opts: opts,
	}

	if err := c.initSession(); err != nil {
		return nil, err
	}
	return c, nil
}

// SoundFormat returns the audio format negotiated with Deezer for this session
// (e.g., "MP3_320" or "MP3_128").
func (c *Client) SoundFormat() string {
	return c.soundFormat
}
