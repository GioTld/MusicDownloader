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
		kind, id, err := ParseURL(tt.url)
		if err != nil {
			t.Errorf("ParseURL(%q) error: %v", tt.url, err)
			continue
		}
		if kind != tt.expectedType {
			t.Errorf("ParseURL(%q) type = %s, want %s", tt.url, kind, tt.expectedType)
		}
		if id == "" {
			t.Errorf("ParseURL(%q) returned empty ID", tt.url)
		}
		t.Logf("ParseURL(%q) -> type: %s, id: %s", tt.url, kind, id)
	}
}

func TestSearchArtist(t *testing.T) {
	client := &Client{http: http.DefaultClient}
	artist, err := client.SearchArtist(context.Background(), "Daft Punk")
	if err != nil {
		t.Fatalf("SearchArtist failed: %v", err)
	}
	if artist.ID != "27" {
		t.Errorf("expected ID 27, got %s", artist.ID)
	}
	if artist.Name != "Daft Punk" {
		t.Errorf("expected name Daft Punk, got %s", artist.Name)
	}
}
