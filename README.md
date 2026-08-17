# MusicDownloader

[![Go Reference](https://pkg.go.dev/badge/github.com/GioTld/MusicDownloader.svg)](https://pkg.go.dev/github.com/GioTld/MusicDownloader)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)](https://golang.org)

MusicDownloader is a Go library and command-line tool for downloading audio tracks, albums, playlists, and full artist discographies from Deezer.

It handles stream decryption (BF_CBC_STRIPE), metadata extraction, concurrent downloads, ID3v2 tagging with cover art, synced `.lrc` lyrics generation, and structured folder layout generation.

## Features

- Downloads tracks, albums, playlists, and full artist discographies by URL or numeric ID.
- Automatically decrypts audio streams using the Deezer Blowfish CBC algorithm.
- Embeds ID3v2 metadata (Title, Artist, Album, Track Number, Disc Number, Year, and high-resolution Front Cover).
- Supports embedding plain-text lyrics (`USLT`) and exporting synced `.lrc` lyrics files (with Deezer & LRCLIB fallback).
- Supports concurrent track downloads with configurable worker limits.
- Supports `context.Context` for graceful cancellation.
- Includes file existence checks to skip already downloaded tracks.
- Preserves organized folder hierarchies based on media type.

## Output Directory Layout

The output structure depends on the resource type being downloaded:

- Single Track:
  `output_dir/Artist - Title.mp3`

- Album:
  `output_dir/(Year) Album Title/01 - Track Title.mp3`

- Playlist:
  `output_dir/Playlist Title/Artist - Track Title.mp3`

- Artist Discography:
  `output_dir/Artist Name/(Year) Album Title/01 - Track Title.mp3`

## Requirements

Deezer requires authentication via an `arl` session cookie.

To obtain your `arl` cookie:
1. Log in to your account on [deezer.com](https://www.deezer.com).
2. Open browser Developer Tools (F12).
3. Navigate to Application -> Cookies -> https://www.deezer.com.
4. Copy the value of the cookie named `arl`.

Optionally add your ARL to your shell profile (`~/.bashrc` or `~/.zshrc`) to avoid passing `-arl` every time:
```bash
export DEEZER_ARL="your_arl_cookie_here"
```

Note: ARL cookies typically expire every 3 months.

## Installation

### As a Command Line Tool

```bash
go install github.com/GioTld/MusicDownloader@latest
```

Or clone and build locally:

```bash
git clone https://github.com/GioTld/MusicDownloader.git
cd MusicDownloader
go build -o music-downloader main.go
```

### As a Go Library

```bash
go get github.com/GioTld/MusicDownloader
```

## Usage

### Go Library Example

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/GioTld/MusicDownloader/deezer"
)

func main() {
	client, err := deezer.New(deezer.Options{
		ARL:          "YOUR_DEEZER_ARL_COOKIE",
		Quality:      deezer.QualityMP3320,
		Concurrency:  3,
		SkipExisting: true,
		EmbedLyrics:  true,
		SaveLRC:      true,
		OnProgress: func(done, total int, track deezer.Track) {
			fmt.Printf("[%d/%d] %s - %s\n", done, total, track.Artist, track.Title)
		},
	})
	if err != nil {
		log.Fatalf("failed to initialize client: %v", err)
	}

	ctx := context.Background()

	// Download by URL (auto-detects track, album, playlist, or artist)
	files, err := client.Download(ctx, "https://www.deezer.com/album/302127", "./music")
	if err != nil {
		log.Fatalf("download failed: %v", err)
	}

	// Or download an artist's discography directly by name search query
	files, err = client.Download(ctx, "Daft Punk", "./music")

	fmt.Printf("Downloaded %d tracks.\n", len(files))
}
```

### CLI Usage

Set the `DEEZER_ARL` environment variable or pass `-arl`:

```bash
# Using environment variable
export DEEZER_ARL="your_arl_cookie_here"
music-downloader https://www.deezer.com/album/302127

# Download an artist discography by name search
music-downloader "Daft Punk"

# Download album with embedded lyrics and synced .lrc files
music-downloader -lyrics -lrc https://www.deezer.com/album/302127

# Using explicit flags
music-downloader -arl "your_arl_cookie_here" -dir ./music -workers 5 https://www.deezer.com/artist/27
```

CLI Flags:

- `-arl`: Deezer ARL cookie (defaults to `DEEZER_ARL` environment variable).
- `-dir`: Target output directory (default: `./downloads`).
- `-quality`: Audio quality, either `320` or `128` (default: `320`).
- `-workers`: Number of parallel track downloads (default: `3`).
- `-skip`: Skip tracks if output file already exists (default: `false`).
- `-lyrics`: Embed plain-text lyrics into MP3 ID3v2 tags (default: `false`).
- `-lrc`: Save synced `.lrc` lyrics file alongside the MP3 (default: `false`).

## Disclaimer

This tool and library are created for educational and personal research purposes only. The authors do not encourage, support, or condone unauthorized downloading or redistribution of copyrighted material. Users are solely responsible for complying with Deezer's Terms of Service and local copyright laws.

## License

MIT
