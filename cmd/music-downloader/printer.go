package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GioTld/MusicDownloader/deezer"
)

type Printer struct {
	mu              sync.Mutex
	useColor        bool
	startTime       time.Time
	totalProcessed  int
	downloadedCount int
	skippedCount    int
	failedCount     int
	totalAlbums     int
}

func NewPrinter() *Printer {
	return &Printer{
		useColor:  shouldUseColor(),
		startTime: time.Now(),
	}
}

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (p *Printer) color(code, text string) string {
	if !p.useColor {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

func (p *Printer) PrintHeader(target deezer.TargetDetails, soundFormat, outDir string, workers int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.totalAlbums = target.TotalAlbums

	boldCyan := "1;36"
	dim := "90"
	bold := "1"

	border := p.color(dim, "════════════════════════════════════════════════════════════")
	title := p.color(boldCyan, "MusicDownloader v1.0.0")

	kindStr := string(target.Kind)
	if len(kindStr) > 0 {
		kindStr = strings.ToUpper(kindStr[:1]) + kindStr[1:]
	}
	targetText := fmt.Sprintf("%s %q", kindStr, target.Name)
	if target.TotalAlbums > 1 {
		targetText += fmt.Sprintf(" (%d albums, %d tracks)", target.TotalAlbums, target.TotalTracks)
	} else if target.TotalTracks > 1 {
		targetText += fmt.Sprintf(" (%d tracks)", target.TotalTracks)
	}

	fmt.Println()
	fmt.Println(title)
	fmt.Println(border)
	fmt.Printf(" %-11s : %s\n", p.color(bold, "Target"), targetText)
	fmt.Printf(" %-11s : %s\n", p.color(bold, "Format"), soundFormat)
	fmt.Printf(" %-11s : %s\n", p.color(bold, "Output Dir"), outDir)
	fmt.Printf(" %-11s : %d\n", p.color(bold, "Workers"), workers)
	fmt.Println(border)
	fmt.Println()
}

func (p *Printer) PrintAlbumStart(albumIndex, totalAlbums int, album deezer.Album) {
	p.mu.Lock()
	defer p.mu.Unlock()

	bold := "1"
	dim := "90"

	header := fmt.Sprintf("Album [%d/%d]: %s", albumIndex, totalAlbums, album.Title)
	if album.Year != "" {
		header = fmt.Sprintf("Album [%d/%d]: %s (%s)", albumIndex, totalAlbums, album.Title, album.Year)
	}
	if len(album.Tracks) > 0 {
		header += fmt.Sprintf(" [%d tracks]", len(album.Tracks))
	}

	divider := p.color(dim, "────────────────────────────────────────────────────────────")
	fmt.Println(p.color(bold, header))
	fmt.Println(divider)
}

func (p *Printer) PrintTrackResult(res deezer.TrackResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.totalProcessed++

	var badge string
	switch res.Status {
	case deezer.StatusDownloaded:
		p.downloadedCount++
		badge = p.color("1;32", "[DONE]")
	case deezer.StatusSkipped:
		p.skippedCount++
		badge = p.color("1;33", "[SKIP]")
	case deezer.StatusFailed:
		p.failedCount++
		badge = p.color("1;31", "[FAIL]")
	}

	durStr := ""
	if res.Track.Duration > 0 {
		m := res.Track.Duration / 60
		s := res.Track.Duration % 60
		durStr = p.color("90", fmt.Sprintf(" (%02d:%02d)", m, s))
	}

	numStr := fmt.Sprintf("[%02d/%02d]", res.TrackIndex, res.TotalTracks)

	extraInfo := ""
	if res.Status == deezer.StatusSkipped {
		extraInfo = p.color("90", " (file exists)")
	} else if res.Status == deezer.StatusFailed && res.Err != nil {
		extraInfo = p.color("31", fmt.Sprintf(" (%v)", res.Err))
	}

	fmt.Printf("  %s %s %s - %s%s%s\n",
		badge,
		numStr,
		res.Track.Artist,
		res.Track.Title,
		durStr,
		extraInfo,
	)
}

func (p *Printer) PrintSummary(outDir string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.startTime).Round(100 * time.Millisecond)
	dim := "90"
	bold := "1"
	border := p.color(dim, "════════════════════════════════════════════════════════════")
	divider := p.color(dim, "────────────────────────────────────────────────────────────")

	fmt.Println()
	fmt.Println(divider)
	fmt.Println(p.color(bold, "Summary"))

	totalStr := fmt.Sprintf("%d tracks", p.totalProcessed)
	if p.totalAlbums > 1 {
		totalStr = fmt.Sprintf("%d tracks across %d albums", p.totalProcessed, p.totalAlbums)
	}

	fmt.Printf("  %-16s : %s\n", "Total processed", totalStr)
	fmt.Printf("  %-16s : %s\n", "Downloaded", p.color("32", fmt.Sprintf("%d tracks", p.downloadedCount)))
	if p.skippedCount > 0 {
		fmt.Printf("  %-16s : %s\n", "Skipped", p.color("33", fmt.Sprintf("%d tracks", p.skippedCount)))
	} else {
		fmt.Printf("  %-16s : %d tracks\n", "Skipped", p.skippedCount)
	}
	if p.failedCount > 0 {
		fmt.Printf("  %-16s : %s\n", "Failed", p.color("31", fmt.Sprintf("%d tracks", p.failedCount)))
	} else {
		fmt.Printf("  %-16s : %d tracks\n", "Failed", p.failedCount)
	}
	fmt.Printf("  %-16s : %s\n", "Elapsed time", elapsed.String())
	fmt.Printf("  %-16s : %s\n", "Destination", outDir)
	fmt.Println(border)
}
