package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GioTld/MusicDownloader/deezer"
	"github.com/GioTld/bubble-motion/pkg/adapter"
	"github.com/GioTld/bubble-motion/pkg/motion"
	"github.com/GioTld/bubble-motion/pkg/spring"
	"github.com/GioTld/bubble-motion/pkg/text"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages sent from Deezer client callbacks into Bubble Tea program.
type (
	tuiTargetMsg deezer.TargetDetails

	tuiAlbumMsg struct {
		index int
		total int
		album deezer.Album
	}

	tuiTrackCompleteMsg deezer.TrackResult

	tuiDoneMsg struct {
		err error
	}
)

type tuiModel struct {
	cancel context.CancelFunc

	// Target info
	target      deezer.TargetDetails
	soundFormat string
	outDir      string
	workers     int
	hasTarget   bool

	// Album progress
	currentAlbumIndex int
	totalAlbums       int
	currentAlbum      deezer.Album

	// Overall track progress
	totalTracks     int
	downloadedCount int
	skippedCount    int
	failedCount     int
	totalProcessed  int

	// Recent activity log (ring buffer of last 5 tracks)
	recentTracks []deezer.TrackResult

	// Motion components
	gradientTitle text.GradientText
	progressBar   adapter.MotionProgress

	// State
	startTime  time.Time
	elapsed    time.Duration
	isDone     bool
	doneErr    error
	width      int
	height     int
}

func newTUIModel(cancel context.CancelFunc, soundFormat, outDir string, workers int) tuiModel {
	sp := spring.Snappy()
	sp.Epsilon = 0.001

	prog := adapter.NewMotionProgress(
		sp,
		progress.WithGradient("#7D56F4", "#00D2FF"),
		progress.WithoutPercentage(),
	)

	gradient := text.NewGradient(
		"★ MUSIC DOWNLOADER ★",
		[]string{"#7D56F4", "#00D2FF", "#A6E22E", "#FF5F87"},
	)

	return tuiModel{
		cancel:        cancel,
		soundFormat:   soundFormat,
		outDir:        outDir,
		workers:       workers,
		startTime:     time.Now(),
		gradientTitle: gradient,
		progressBar:   prog,
		width:         80,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return motion.Tick()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "enter":
			if m.isDone {
				return m, tea.Quit
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		barWidth := msg.Width - 10
		if barWidth > 60 {
			barWidth = 60
		}
		if barWidth < 20 {
			barWidth = 20
		}
		m.progressBar.Progress.Width = barWidth
		return m, nil

	case motion.TickMsg:
		m.elapsed = time.Since(m.startTime).Round(100 * time.Millisecond)
		m.gradientTitle, _ = m.gradientTitle.Update(msg)
		m.progressBar, _ = m.progressBar.Update(msg)

		if !m.isDone || m.progressBar.Active() {
			return m, motion.Tick()
		}
		return m, nil

	case tuiTargetMsg:
		m.target = deezer.TargetDetails(msg)
		m.totalTracks = m.target.TotalTracks
		m.totalAlbums = m.target.TotalAlbums
		m.hasTarget = true
		return m, nil

	case tuiAlbumMsg:
		m.currentAlbumIndex = msg.index
		m.totalAlbums = msg.total
		m.currentAlbum = msg.album
		return m, nil

	case tuiTrackCompleteMsg:
		m.totalProcessed++
		switch msg.Status {
		case deezer.StatusDownloaded:
			m.downloadedCount++
		case deezer.StatusSkipped:
			m.skippedCount++
		case deezer.StatusFailed:
			m.failedCount++
		}

		// Keep only the last 5 items
		m.recentTracks = append(m.recentTracks, deezer.TrackResult(msg))
		if len(m.recentTracks) > 5 {
			m.recentTracks = m.recentTracks[len(m.recentTracks)-5:]
		}

		if m.totalTracks > 0 {
			fraction := float64(m.totalProcessed) / float64(m.totalTracks)
			m.progressBar, _ = m.progressBar.SetPercent(fraction)
		}
		return m, nil

	case tuiDoneMsg:
		m.isDone = true
		m.doneErr = msg.err
		m.elapsed = time.Since(m.startTime).Round(100 * time.Millisecond)
		if m.totalTracks > 0 {
			m.progressBar, _ = m.progressBar.SetPercent(1.0)
		}
		return m, tea.Quit
	}

	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5A5A7A")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#626272")).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0"))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00D2FF")).Bold(true)
	purpleBadge := lipgloss.NewStyle().Background(lipgloss.Color("#7D56F4")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1).Bold(true)
	cyanBadge := lipgloss.NewStyle().Background(lipgloss.Color("#00D2FF")).Foreground(lipgloss.Color("#1A1A24")).Padding(0, 1).Bold(true)
	greenBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E22E")).Bold(true)
	yellowBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6DB74")).Bold(true)
	redBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("#F92672")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#626272"))

	// 1. Header
	b.WriteString("\n ")
	b.WriteString(m.gradientTitle.View())
	b.WriteString("  ")
	b.WriteString(purpleBadge.Render(m.soundFormat))
	b.WriteString(" ")
	b.WriteString(cyanBadge.Render(fmt.Sprintf("%d workers", m.workers)))
	b.WriteString("\n\n")

	// 2. Target info card
	if m.hasTarget {
		targetKind := strings.ToUpper(string(m.target.Kind[:1])) + string(m.target.Kind[1:])
		targetDesc := fmt.Sprintf("%s %s", labelStyle.Render(targetKind+":"), accentStyle.Render(fmt.Sprintf("%q", m.target.Name)))
		if m.totalAlbums > 1 {
			targetDesc += dimStyle.Render(fmt.Sprintf(" (%d albums, %d tracks)", m.totalAlbums, m.totalTracks))
		} else if m.totalTracks > 1 {
			targetDesc += dimStyle.Render(fmt.Sprintf(" (%d tracks)", m.totalTracks))
		}
		b.WriteString(fmt.Sprintf(" %s\n", targetDesc))
		b.WriteString(fmt.Sprintf(" %s %s\n\n", labelStyle.Render("Destination:"), valStyle.Render(m.outDir)))
	}

	// 3. Current Album indicator
	if m.currentAlbum.Title != "" {
		albumHeader := fmt.Sprintf("💿 Album [%d/%d]: %s", m.currentAlbumIndex, m.totalAlbums, m.currentAlbum.Title)
		if m.currentAlbum.Year != "" {
			albumHeader += fmt.Sprintf(" (%s)", m.currentAlbum.Year)
		}
		b.WriteString(" ")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E22E")).Bold(true).Render(albumHeader))
		b.WriteString("\n")
	}

	// 4. Spring Progress Bar
	if m.totalTracks > 0 {
		pct := int(m.progressBar.Percent() * 100)
		progText := fmt.Sprintf(" %s  %s\n",
			m.progressBar.View(),
			accentStyle.Render(fmt.Sprintf("%3d%% [%d/%d]", pct, m.totalProcessed, m.totalTracks)),
		)
		b.WriteString(progText)
	}

	// 5. Counters bar
	counters := fmt.Sprintf(" %s %s  •  %s %s  •  %s %s  •  %s %s",
		greenBadge.Render("✓ Done:"), valStyle.Render(fmt.Sprintf("%d", m.downloadedCount)),
		yellowBadge.Render("⇥ Skip:"), valStyle.Render(fmt.Sprintf("%d", m.skippedCount)),
		redBadge.Render("✕ Fail:"), valStyle.Render(fmt.Sprintf("%d", m.failedCount)),
		labelStyle.Render("⏱ Elapsed:"), dimStyle.Render(m.elapsed.String()),
	)
	b.WriteString(counters)
	b.WriteString("\n\n")

	// 6. Recent Tracks Feed
	if len(m.recentTracks) > 0 {
		var feed strings.Builder
		for _, r := range m.recentTracks {
			var badge string
			switch r.Status {
			case deezer.StatusDownloaded:
				badge = greenBadge.Render("[DONE]")
			case deezer.StatusSkipped:
				badge = yellowBadge.Render("[SKIP]")
			case deezer.StatusFailed:
				badge = redBadge.Render("[FAIL]")
			}

			dur := ""
			if r.Track.Duration > 0 {
				dur = dimStyle.Render(fmt.Sprintf(" (%02d:%02d)", r.Track.Duration/60, r.Track.Duration%60))
			}

			extra := ""
			if r.Status == deezer.StatusSkipped {
				extra = dimStyle.Render(" (file exists)")
			} else if r.Status == deezer.StatusFailed && r.Err != nil {
				extra = redBadge.Render(fmt.Sprintf(" (%v)", r.Err))
			}

			trackLine := fmt.Sprintf("%s %s - %s%s%s\n",
				badge,
				valStyle.Render(r.Track.Artist),
				accentStyle.Render(r.Track.Title),
				dur,
				extra,
			)
			feed.WriteString(trackLine)
		}
		b.WriteString(borderStyle.Render(strings.TrimRight(feed.String(), "\n")))
		b.WriteString("\n\n")
	}

	// 7. Footer / Status
	if m.isDone {
		summaryStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A6E22E"))
		if m.doneErr != nil {
			summaryStyle = summaryStyle.Foreground(lipgloss.Color("#F92672"))
			b.WriteString(summaryStyle.Render(fmt.Sprintf(" ✕ Finished with error: %v\n", m.doneErr)))
		} else {
			b.WriteString(summaryStyle.Render(fmt.Sprintf(" ✓ All downloads complete in %s!\n", m.elapsed)))
		}
	} else {
		b.WriteString(dimStyle.Render(" Press 'q' or 'esc' to cancel download • Powered by bubble-motion\n"))
	}

	return b.String()
}
