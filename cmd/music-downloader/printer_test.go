package main

import (
	"os"
	"testing"

	"github.com/GioTld/MusicDownloader/deezer"
)

func TestPrinterFormatting(t *testing.T) {
	p := NewPrinter()
	p.useColor = false // disable ANSI for deterministic assertion

	target := deezer.TargetDetails{
		Kind:        deezer.TypeArtist,
		Name:        "Chris Isaak",
		TotalAlbums: 2,
		TotalTracks: 20,
	}

	// Test header printing doesn't panic
	p.PrintHeader(target, "MP3_320", "./downloads", 3)

	// Test album start printing
	album := deezer.Album{Title: "Silvertone", Year: "1985", Tracks: make([]deezer.Track, 10)}
	p.PrintAlbumStart(1, 2, album)

	// Test track results
	resDone := deezer.TrackResult{
		Track:       deezer.Track{Title: "Diddley Daddy", Artist: "Chris Isaak", Duration: 234},
		Status:      deezer.StatusDownloaded,
		TrackIndex:  1,
		TotalTracks: 10,
	}
	resSkip := deezer.TrackResult{
		Track:       deezer.Track{Title: "Back Seat", Artist: "Chris Isaak", Duration: 195},
		Status:      deezer.StatusSkipped,
		TrackIndex:  2,
		TotalTracks: 10,
	}

	p.PrintTrackResult(resDone)
	p.PrintTrackResult(resSkip)
	p.PrintSummary("./downloads")

	if p.downloadedCount != 1 {
		t.Errorf("expected downloadedCount 1, got %d", p.downloadedCount)
	}
	if p.skippedCount != 1 {
		t.Errorf("expected skippedCount 1, got %d", p.skippedCount)
	}
	if p.totalProcessed != 2 {
		t.Errorf("expected totalProcessed 2, got %d", p.totalProcessed)
	}
}

func TestShouldUseColorWithNOCOLOR(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	if shouldUseColor() {
		t.Error("expected shouldUseColor() to be false when NO_COLOR is set")
	}
}
