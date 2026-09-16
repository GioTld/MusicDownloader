// music-downloader downloads music from Deezer by track, album, playlist, or artist URL.
//
// Authentication requires a valid ARL cookie set via the -arl flag or DEEZER_ARL env variable.
// Obtain the ARL from browser DevTools: Application → Cookies → deezer.com → arl.
//
// A config file is auto-loaded from ~/.config/music-downloader/config.toml if it exists.
// CLI flags override any config file values.
//
// Usage:
//
//	music-downloader [flags] <deezer-url>
//
// Examples:
//
//	music-downloader https://www.deezer.com/track/3135556
//	music-downloader https://www.deezer.com/album/302127 -dir ./music
//	music-downloader https://www.deezer.com/artist/27 -workers 5
//	music-downloader -sync https://www.deezer.com/playlist/1234
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"syscall"

	"github.com/GioTld/MusicDownloader/deezer"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\ndownload cancelled")
			os.Exit(0)
		}
		var authErr *deezer.ErrAuth
		if errors.As(err, &authErr) {
			log.Fatalf("authentication failed: %v\nCheck your ARL cookie.", err)
		}
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	// ── Config file (lowest priority) ────────────────────────────────────
	// We do a first-pass parse just for -config so we know which file to load.
	firstPass := flag.NewFlagSet("pre", flag.ContinueOnError)
	firstPass.SetOutput(io.Discard)
	configPath := firstPass.String("config", "", "")
	_ = firstPass.Parse(os.Args[1:])

	cfg, _, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// ── CLI flags (highest priority) ──────────────────────────────────────
	fs := flag.NewFlagSet("music-downloader", flag.ExitOnError)

	arl          := fs.String("arl",        firstNonEmpty(os.Getenv("DEEZER_ARL"), cfg.ARL), "Deezer ARL cookie (or set DEEZER_ARL env var)")
	dir          := fs.String("dir",        stringDefault(cfg.Dir, "./downloads"),            "output directory")
	quality      := fs.String("quality",    stringDefault(cfg.Quality, "320"),                "audio quality: 128, 320, or flac")
	workers      := fs.Int("workers",       intDefault(cfg.Workers, 8),                       "number of parallel downloads")
	skip         := fs.Bool("skip",         cfg.Skip,                                         "skip tracks whose output file already exists")
	sync         := fs.Bool("sync",         cfg.Sync,                                         "sync mode: skip valid files, re-download corrupt/incomplete ones")
	embedLyrics  := fs.Bool("lyrics",       cfg.Lyrics,                                       "embed lyrics into MP3/FLAC tags")
	saveLRC      := fs.Bool("lrc",          cfg.LRC,                                          "save synced .lrc lyrics file alongside audio")
	tuiFlag      := fs.Bool("tui",          boolDefault(cfg.TUI, true),                       "enable animated interactive TUI")
	cpuProfile   := fs.String("cpuprofile", "",                                               "write cpu profile to file")
	memProfile   := fs.String("memprofile", "",                                               "write memory profile to file")
	traceProfile := fs.String("trace",      "",                                               "write execution trace to file")
	fs.String("config", *configPath, "path to config.toml (default: ~/.config/music-downloader/config.toml)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	// ── Profiling ─────────────────────────────────────────────────────────
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			return fmt.Errorf("create cpu profile: %w", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("start cpu profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}
	if *traceProfile != "" {
		f, err := os.Create(*traceProfile)
		if err != nil {
			return fmt.Errorf("create trace profile: %w", err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			return fmt.Errorf("start trace: %w", err)
		}
		defer trace.Stop()
	}
	if *memProfile != "" {
		defer func() {
			f, err := os.Create(*memProfile)
			if err != nil {
				log.Printf("create mem profile: %v", err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Printf("write heap profile: %v", err)
			}
		}()
	}

	// ── Validation ────────────────────────────────────────────────────────
	if *arl == "" {
		fs.Usage()
		return errors.New("ARL cookie is required (-arl flag, DEEZER_ARL env var, or config.toml)")
	}
	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		return errors.New("provide a Deezer URL as the last argument")
	}
	rawURL := args[0]

	// ── Quality ───────────────────────────────────────────────────────────
	var q deezer.Quality
	switch *quality {
	case "flac", "FLAC":
		q = deezer.QualityFLAC
	case "128":
		q = deezer.QualityMP3128
	default:
		q = deezer.QualityMP3320
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	useTUI := shouldUseColor() && *tuiFlag

	if useTUI {
		downloadCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		model := newTUIModel(cancel, string(q), *dir, *workers)
		p := tea.NewProgram(model, tea.WithContext(downloadCtx))

		client, err := deezer.New(deezer.Options{
			ARL:          *arl,
			Quality:      q,
			Concurrency:  *workers,
			SkipExisting: *skip || *sync,
			SyncMode:     *sync,
			EmbedLyrics:  *embedLyrics,
			SaveLRC:      *saveLRC,
			OnTargetResolved: func(target deezer.TargetDetails) {
				p.Send(tuiTargetMsg(target))
			},
			OnAlbumStart: func(albumIndex, totalAlbums int, album deezer.Album) {
				p.Send(tuiAlbumMsg{index: albumIndex, total: totalAlbums, album: album})
			},
			OnTrackComplete: func(res deezer.TrackResult) {
				p.Send(tuiTrackCompleteMsg(res))
			},
		})
		if err != nil {
			return fmt.Errorf("init: %w", err)
		}

		var downloadErr error
		downloadDone := make(chan struct{})
		go func() {
			defer close(downloadDone)
			_, downloadErr = client.Download(downloadCtx, rawURL, *dir)
			p.Send(tuiDoneMsg{err: downloadErr})
		}()

		if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
			return fmt.Errorf("tui: %w", err)
		}

		<-downloadDone

		if downloadErr != nil {
			if errors.Is(downloadErr, context.Canceled) {
				return downloadErr
			}
			return fmt.Errorf("download failed: %w", downloadErr)
		}
		if errors.Is(downloadCtx.Err(), context.Canceled) {
			return downloadCtx.Err()
		}
		return nil
	}

	printer := NewPrinter()

	client, err := deezer.New(deezer.Options{
		ARL:          *arl,
		Quality:      q,
		Concurrency:  *workers,
		SkipExisting: *skip || *sync,
		SyncMode:     *sync,
		EmbedLyrics:  *embedLyrics,
		SaveLRC:      *saveLRC,
		OnTargetResolved: func(target deezer.TargetDetails) {
			printer.PrintHeader(target, string(q), *dir, *workers)
		},
		OnAlbumStart: func(albumIndex, totalAlbums int, album deezer.Album) {
			printer.PrintAlbumStart(albumIndex, totalAlbums, album)
		},
		OnTrackComplete: func(res deezer.TrackResult) {
			printer.PrintTrackResult(res)
		},
	})
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	_, err = client.Download(ctx, rawURL, *dir)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	printer.PrintSummary(*dir)
	return nil
}

// firstNonEmpty returns the first non-empty string from the list.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// stringDefault returns s if non-empty, otherwise fallback.
func stringDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// intDefault returns n if > 0, otherwise fallback.
func intDefault(n, fallback int) int {
	if n > 0 {
		return n
	}
	return fallback
}
