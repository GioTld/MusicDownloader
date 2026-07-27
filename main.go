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
	"syscall"

	"github.com/GioPelao2/MusicDownloader/deezer"
)

func main() {
	arl := flag.String("arl", os.Getenv("DEEZER_ARL"), "Deezer ARL cookie (or set DEEZER_ARL env var)")
	dir := flag.String("dir", "./downloads", "output directory")
	quality := flag.String("quality", "320", "audio quality: 128 or 320")
	workers := flag.Int("workers", 3, "number of parallel downloads")
	skip := flag.Bool("skip", false, "skip tracks whose output file already exists")
	flag.Parse()

	if *arl == "" {
		fmt.Fprintln(os.Stderr, "error: ARL cookie is required (-arl flag or DEEZER_ARL env var)")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: provide a Deezer URL as the last argument")
		flag.Usage()
		os.Exit(1)
	}
	rawURL := args[0]

	q := deezer.QualityMP3320
	if *quality == "128" {
		q = deezer.QualityMP3128
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := deezer.New(deezer.Options{
		ARL:          *arl,
		Quality:      q,
		Concurrency:  *workers,
		SkipExisting: *skip,
		OnProgress: func(done, total int, track deezer.Track) {
			fmt.Printf("[%d/%d] %s - %s\n", done, total, track.Artist, track.Title)
		},
	})
	if err != nil {
		var authErr *deezer.ErrAuth
		if errors.As(err, &authErr) {
			log.Fatalf("authentication failed: %v\nCheck your ARL cookie.", err)
		}
		log.Fatalf("init: %v", err)
	}

	fmt.Printf("session ready — format: %s\n", client.SoundFormat())

	files, err := client.Download(ctx, rawURL, *dir)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\ndownload cancelled")
			os.Exit(0)
		}
		log.Fatalf("download failed: %v", err)
	}

	fmt.Printf("\n%d file(s) saved to %s\n", len(files), *dir)
}
