// music-downloader downloads music from Deezer by track, album, playlist, or artist URL.
//
// Authentication requires a valid ARL cookie set via the -arl flag or DEEZER_ARL env variable.
// Obtain the ARL from browser DevTools: Application → Cookies → deezer.com → arl.
//
// Usage:
//
//	music-downloader [flags] <deezer-url>
//
// Examples:
//
//	music-downloader -arl $DEEZER_ARL https://www.deezer.com/track/3135556
//	music-downloader -arl $DEEZER_ARL https://www.deezer.com/album/302127 -dir ./music
//	music-downloader -arl $DEEZER_ARL https://www.deezer.com/artist/27 -workers 5
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"syscall"

	"github.com/GioTld/MusicDownloader/deezer"
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
	arl := flag.String("arl", os.Getenv("DEEZER_ARL"), "Deezer ARL cookie (or set DEEZER_ARL env var)")
	dir := flag.String("dir", "./downloads", "output directory")
	quality := flag.String("quality", "320", "audio quality: 128 or 320")
	workers := flag.Int("workers", 8, "number of parallel downloads")
	skip := flag.Bool("skip", false, "skip tracks whose output file already exists")
	embedLyrics := flag.Bool("lyrics", false, "embed lyrics into MP3 ID3v2 tags")
	saveLRC := flag.Bool("lrc", false, "save synced .lrc lyrics file alongside MP3")
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write memory profile to file")
	traceProfile := flag.String("trace", "", "write execution trace to file")
	flag.Parse()

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

	if *arl == "" {
		flag.Usage()
		return errors.New("ARL cookie is required (-arl flag or DEEZER_ARL env var)")
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		return errors.New("provide a Deezer URL as the last argument")
	}
	rawURL := args[0]

	q := deezer.QualityMP3320
	if *quality == "128" {
		q = deezer.QualityMP3128
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	printer := NewPrinter()

	client, err := deezer.New(deezer.Options{
		ARL:          *arl,
		Quality:      q,
		Concurrency:  *workers,
		SkipExisting: *skip,
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
