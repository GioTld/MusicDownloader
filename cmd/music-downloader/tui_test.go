package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GioTld/MusicDownloader/deezer"
	"github.com/GioTld/bubble-motion/pkg/motion"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIModelLifecycle(t *testing.T) {
	cancelled := false
	cancel := func() {
		cancelled = true
	}

	m := newTUIModel(cancel, "FLAC", "./test_downloads", 8)

	// 1. Init should return a motion tick command
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a non-nil tea.Cmd")
	}

	// 2. Window resize
	mModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = mModel.(tuiModel)
	if m.width != 100 || m.height != 30 {
		t.Errorf("expected 100x30, got %dx%d", m.width, m.height)
	}
	if m.progressBar.Progress.Width != 60 {
		t.Errorf("expected clamped bar width 60, got %d", m.progressBar.Progress.Width)
	}

	// 3. Target resolved
	mModel, _ = m.Update(tuiTargetMsg(deezer.TargetDetails{
		Kind:        deezer.TypeAlbum,
		Name:        "Random Access Memories",
		TotalAlbums: 1,
		TotalTracks: 13,
	}))
	m = mModel.(tuiModel)
	if !m.hasTarget || m.totalTracks != 13 || m.target.Name != "Random Access Memories" {
		t.Errorf("target not updated correctly: %+v", m.target)
	}

	// 4. Album start
	mModel, _ = m.Update(tuiAlbumMsg{
		index: 1,
		total: 1,
		album: deezer.Album{Title: "Random Access Memories", Year: "2013"},
	})
	m = mModel.(tuiModel)
	if m.currentAlbum.Title != "Random Access Memories" || m.currentAlbumIndex != 1 {
		t.Errorf("album not updated correctly: %+v", m.currentAlbum)
	}

	// 5. Track complete: Downloaded
	mModel, _ = m.Update(tuiTrackCompleteMsg(deezer.TrackResult{
		Track:      deezer.Track{Title: "Give Life Back to Music", Artist: "Daft Punk", Duration: 275},
		Status:     deezer.StatusDownloaded,
		TrackIndex: 1,
	}))
	m = mModel.(tuiModel)
	if m.downloadedCount != 1 || m.totalProcessed != 1 {
		t.Errorf("expected 1 downloaded track, got %d", m.downloadedCount)
	}

	// 6. Track complete: Skipped
	mModel, _ = m.Update(tuiTrackCompleteMsg(deezer.TrackResult{
		Track:      deezer.Track{Title: "Giorgio by Moroder", Artist: "Daft Punk", Duration: 544},
		Status:     deezer.StatusSkipped,
		TrackIndex: 2,
	}))
	m = mModel.(tuiModel)
	if m.skippedCount != 1 || m.totalProcessed != 2 {
		t.Errorf("expected 1 skipped track, got %d", m.skippedCount)
	}

	// 7. Tick update advances gradient and spring
	mModel, _ = m.Update(motion.TickMsg{Time: time.Now()})
	m = mModel.(tuiModel)

	// 8. View output checks
	view := m.View()
	if !strings.Contains(view, "MUSIC DOWNLOADER") {
		t.Error("expected View to contain title")
	}
	if !strings.Contains(view, "Random Access Memories") {
		t.Error("expected View to contain target/album name")
	}
	if !strings.Contains(view, "Give Life Back to Music") {
		t.Error("expected View to contain recent track title")
	}
	if !strings.Contains(view, "FLAC") {
		t.Error("expected View to contain sound format badge")
	}

	// 9. Quit key triggers cancel
	_, quitCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		t.Error("expected q to return tea.Quit")
	}
	if !cancelled {
		t.Error("expected cancel() to be called on 'q'")
	}

	// 10. Done message
	mModel, doneCmd := m.Update(tuiDoneMsg{err: nil})
	m = mModel.(tuiModel)
	if !m.isDone || doneCmd == nil {
		t.Error("expected isDone to be true and quitCmd on tuiDoneMsg")
	}

	// View when done
	doneView := m.View()
	if !strings.Contains(doneView, "complete") {
		t.Error("expected View to contain completion message")
	}
}

func TestTUIModelErrorDone(t *testing.T) {
	m := newTUIModel(func() {}, "MP3_320", "./downloads", 4)
	mModel, _ := m.Update(tuiDoneMsg{err: errors.New("network failure")})
	m = mModel.(tuiModel)

	view := m.View()
	if !strings.Contains(view, "network failure") {
		t.Errorf("expected error in View, got: %s", view)
	}
}

func TestTUIModelContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := newTUIModel(cancel, "MP3_320", "./downloads", 4)

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if ctx.Err() != context.Canceled {
		t.Error("expected context to be cancelled on Ctrl+C")
	}
}
