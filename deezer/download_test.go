package deezer

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Normal Name", "Normal Name"},
		{"AC/DC", "AC_DC"},
		{"What? (Live: 1999)", "What_ (Live_ 1999)"},
		{"File with trailing dots...", "File with trailing dots"},
		{"File with trailing spaces   ", "File with trailing spaces"},
		{"Special chars: <>:\"/\\|?*", "Special chars_ _________"},
		{"", "unknown"},
		{"...", "unknown"},
	}

	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.expected {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFilenameAndDirHelpers(t *testing.T) {
	trackWithNum := Track{
		Title:       "Test Track",
		Artist:      "Test Artist",
		TrackNumber: 3,
	}
	trackNoNum := Track{
		Title:       "Another Track",
		Artist:      "Test Artist",
		TrackNumber: 0,
	}

	if got := albumFilename(trackWithNum); got != "03 - Test Track.mp3" {
		t.Errorf("albumFilename(trackWithNum) = %q, want %q", got, "03 - Test Track.mp3")
	}
	if got := albumFilename(trackNoNum); got != "Another Track.mp3" {
		t.Errorf("albumFilename(trackNoNum) = %q, want %q", got, "Another Track.mp3")
	}
	if got := standaloneFilename(trackWithNum); got != "Test Artist - Test Track.mp3" {
		t.Errorf("standaloneFilename = %q, want %q", got, "Test Artist - Test Track.mp3")
	}

	albumWithYear := Album{Title: "Greatest Hits", Year: "2010"}
	albumNoYear := Album{Title: "Self Titled", Year: ""}
	if got := albumDirName(albumWithYear); got != "(2010) Greatest Hits" {
		t.Errorf("albumDirName(albumWithYear) = %q, want %q", got, "(2010) Greatest Hits")
	}
	if got := albumDirName(albumNoYear); got != "Self Titled" {
		t.Errorf("albumDirName(albumNoYear) = %q, want %q", got, "Self Titled")
	}
}

// mockDeezerRoundTripper simulates Deezer endpoints in memory without any external network calls.
type mockDeezerRoundTripper struct {
	audioPayload []byte
}

func (m *mockDeezerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	urlStr := req.URL.String()

	// 1. Private API: getUserData, song.getData, song.getListByAlbum
	if strings.HasPrefix(urlStr, privateAPIURL) {
		method := req.URL.Query().Get("method")
		switch method {
		case "deezer.getUserData":
			respBody := `{
				"results": {
					"checkForm": "mock_check_form_token",
					"USER": {
						"OPTIONS": {
							"license_token": "mock_license_token",
							"web_sound_quality": {"high": true}
						}
					}
				},
				"error": []
			}`
			return jsonResponse(http.StatusOK, respBody), nil

		case "song.getData":
			respBody := `{
				"results": {
					"SNG_ID": "111",
					"SNG_TITLE": "Mock Song",
					"ART_NAME": "Mock Artist",
					"ALB_TITLE": "Mock Album",
					"TRACK_TOKEN": "mock_track_token",
					"MD5_ORIGIN": "dummy_md5",
					"MEDIA_VERSION": "1",
					"DURATION": "180",
					"TRACK_NUMBER": "1",
					"DISK_NUMBER": "1",
					"PHYSICAL_RELEASE_DATE": "2024-01-01"
				},
				"error": []
			}`
			return jsonResponse(http.StatusOK, respBody), nil

		case "song.getListByAlbum":
			respBody := `{
				"results": {
					"data": [
						{
							"SNG_ID": "111",
							"SNG_TITLE": "Mock Song 1",
							"ART_NAME": "Mock Artist",
							"ALB_TITLE": "Mock Album",
							"TRACK_TOKEN": "mock_track_token_1",
							"DURATION": "180",
							"TRACK_NUMBER": "1",
							"DISK_NUMBER": "1",
							"PHYSICAL_RELEASE_DATE": "2024-01-01"
						}
					]
				},
				"error": []
			}`
			return jsonResponse(http.StatusOK, respBody), nil

		case "song.getLyrics":
			respBody := `{
				"results": {
					"LYRICS_TEXT": "Mock lyrics text",
					"LYRICS_SYNC_JSON": [{"lrc_timestamp": "[00:01.00]", "line": "Mock synced line"}]
				},
				"error": []
			}`
			return jsonResponse(http.StatusOK, respBody), nil
		}
	}

	// 2. Media API: get_url
	if strings.HasPrefix(urlStr, mediaAPIURL) {
		respBody := `{
			"data": [
				{
					"media": [
						{
							"sources": [{"url": "https://stream.mock.deezer.com/audio.mp3"}]
						}
					]
				}
			]
		}`
		return jsonResponse(http.StatusOK, respBody), nil
	}

	// 3. Audio stream
	if strings.HasPrefix(urlStr, "https://stream.mock.deezer.com/audio.mp3") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(m.audioPayload)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}

	// 4. Public API: /album/..., /artist/...
	if strings.HasPrefix(urlStr, publicAPIURL) {
		if strings.Contains(urlStr, "/album/") {
			respBody := `{
				"id": 222,
				"title": "Mock Album",
				"release_date": "2024-01-01",
				"cover_medium": "https://e-cdns-images.dzcdn.net/images/cover/mock_cover_id/500x500.jpg",
				"artist": {"name": "Mock Artist"}
			}`
			return jsonResponse(http.StatusOK, respBody), nil
		}
	}

	// 5. Cover art
	if strings.HasPrefix(urlStr, coverBaseURL) {
		dummyJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(dummyJPEG)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("404 Not Found")),
		Request:    req,
	}, nil
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}
}

func TestClientOfflineDownloadTrackAndAlbum(t *testing.T) {
	// Prepare audio payload encrypted with BF_CBC_STRIPE
	rawAudio := make([]byte, 4096)
	rng := rand.New(rand.NewSource(1234))
	rng.Read(rawAudio)
	key := deriveKey("111")
	encryptedAudio, err := encryptBF_CBC_STRIPE(rawAudio, key)
	if err != nil {
		t.Fatalf("failed to encrypt mock audio: %v", err)
	}

	mockRT := &mockDeezerRoundTripper{audioPayload: encryptedAudio}

	var resolvedTarget TargetDetails
	var completedResults []TrackResult

	client, err := New(Options{
		ARL:          "mock_arl_1234567890",
		Quality:      QualityMP3320,
		Concurrency:  2,
		SkipExisting: true,
		EmbedLyrics:  true,
		SaveLRC:      true,
		Transport:    mockRT,
		OnTargetResolved: func(target TargetDetails) {
			resolvedTarget = target
		},
		OnTrackComplete: func(res TrackResult) {
			completedResults = append(completedResults, res)
		},
	})
	if err != nil {
		t.Fatalf("failed to initialize client with mock transport: %v", err)
	}

	tmpDir := t.TempDir()

	// 1. Test DownloadTrack
	trackPath, err := client.DownloadTrack(context.Background(), "111", tmpDir)
	if err != nil {
		t.Fatalf("DownloadTrack failed: %v", err)
	}
	if _, err := os.Stat(trackPath); err != nil {
		t.Fatalf("expected downloaded track file at %s: %v", trackPath, err)
	}

	// Verify LRC file was saved
	lrcPath := strings.TrimSuffix(trackPath, ".mp3") + ".lrc"
	if _, err := os.Stat(lrcPath); err != nil {
		t.Errorf("expected LRC file at %s: %v", lrcPath, err)
	}

	if resolvedTarget.Kind != TypeTrack {
		t.Errorf("TargetDetails.Kind = %s, want %s", resolvedTarget.Kind, TypeTrack)
	}
	if len(completedResults) != 1 || completedResults[0].Status != StatusDownloaded {
		t.Fatalf("expected 1 downloaded result, got %+v", completedResults)
	}

	// 2. Test SkipExisting
	completedResults = nil
	skipPath, err := client.DownloadTrack(context.Background(), "111", tmpDir)
	if err != nil {
		t.Fatalf("DownloadTrack with SkipExisting failed: %v", err)
	}
	if skipPath != trackPath {
		t.Errorf("skipPath = %s, want %s", skipPath, trackPath)
	}
	if len(completedResults) != 1 || completedResults[0].Status != StatusSkipped {
		t.Fatalf("expected 1 skipped result, got %+v", completedResults)
	}

	// 3. Test DownloadAlbum
	completedResults = nil
	albumPaths, err := client.DownloadAlbum(context.Background(), "222", tmpDir)
	if err != nil {
		t.Fatalf("DownloadAlbum failed: %v", err)
	}
	if len(albumPaths) != 1 {
		t.Fatalf("expected 1 album track, got %d", len(albumPaths))
	}
}

func TestClientDownloadCancellation(t *testing.T) {
	rawAudio := make([]byte, 2048)
	key := deriveKey("111")
	encryptedAudio, _ := encryptBF_CBC_STRIPE(rawAudio, key)
	mockRT := &mockDeezerRoundTripper{audioPayload: encryptedAudio}

	client, err := New(Options{
		ARL:       "mock_arl",
		Transport: mockRT,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tmpDir := t.TempDir()
	_, err = client.DownloadTrack(ctx, "111", tmpDir)
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}
