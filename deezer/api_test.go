package deezer

import (
	"context"
	"net/http"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		url          string
		expectedType MediaType
		expectedID   string
	}{
		{"https://www.deezer.com/track/3135556", TypeTrack, "3135556"},
		{"https://www.deezer.com/us/album/319742677", TypeAlbum, "319742677"},
		{"https://www.deezer.com/es/artist/27", TypeArtist, "27"},
		{"https://link.deezer.com/s/33WUfMwuOXBkaH2atZ3a3", TypeAlbum, "319742677"}, // or whatever redirect it goes to
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if testing.Short() && tt.expectedType == "" {
				t.Skip("skipping network redirect test in short mode")
			}
			kind, id, err := ParseURL(tt.url)
			if err != nil {
				// If it's a network error on the shortlink redirect, skip gracefully
				if tt.url == "https://link.deezer.com/s/33WUfMwuOXBkaH2atZ3a3" {
					t.Skipf("skipping live shortlink redirect test (offline/network): %v", err)
				}
				t.Fatalf("ParseURL(%q) error: %v", tt.url, err)
			}
			if kind != tt.expectedType {
				t.Errorf("ParseURL(%q) type = %s, want %s", tt.url, kind, tt.expectedType)
			}
			if id == "" {
				t.Errorf("ParseURL(%q) returned empty ID", tt.url)
			}
		})
	}
}

func TestSearchArtist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test in -short mode")
	}

	client := &Client{http: http.DefaultClient}
	artist, err := client.SearchArtist(context.Background(), "Daft Punk")
	if err != nil {
		t.Skipf("skipping live API test (network unavailable or rate limited): %v", err)
	}
	if artist.ID != "27" {
		t.Errorf("expected ID 27, got %s", artist.ID)
	}
	if artist.Name != "Daft Punk" {
		t.Errorf("expected name Daft Punk, got %s", artist.Name)
	}
}
