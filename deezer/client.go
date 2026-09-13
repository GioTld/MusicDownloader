package deezer

import (
	"net/http"
	"time"
)

const (
	privateAPIURL = "https://www.deezer.com/ajax/gw-light.php"
	mediaAPIURL   = "https://media.deezer.com/v1/get_url"
	publicAPIURL  = "https://api.deezer.com"
	coverBaseURL  = "https://e-cdns-images.dzcdn.net/images/cover"

	// blowfishSecret is hardcoded in Deezer's web player JavaScript bundle.
	blowfishSecret = "g4el58wc0zvf9na1"

	chunkSize    = 2048
	encryptEvery = 3

	userAgent = "Mozilla/5.0 (X11; Linux i686; rv:135.0) Gecko/20100101 Firefox/135.0"
)

// blowfishIV is the fixed CBC IV used by BF_CBC_STRIPE; it resets for each encrypted chunk.
var blowfishIV = []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}

func (c *Client) initSession() error {
	licenseToken, quality, err := c.getUserData()
	if err != nil {
		return err
	}
	c.licenseToken = licenseToken
	c.soundFormat = c.resolveSoundFormat(quality)
	return nil
}

func (c *Client) resolveSoundFormat(quality map[string]any) string {
	if c.opts.Quality == QualityFLAC {
		if lossless, _ := quality["lossless"].(bool); lossless {
			return "FLAC"
		}
		// Fallback to high if lossless is not available on this account
		if high, _ := quality["high"].(bool); high {
			return "MP3_320"
		}
	}
	if c.opts.Quality == QualityMP3320 {
		if high, _ := quality["high"].(bool); high {
			return "MP3_320"
		}
	}
	return "MP3_128"
}

func (c *Client) newRequest(method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://www.deezer.com")
	req.Header.Set("Referer", "https://www.deezer.com/login")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("DNT", "1")
	return req, nil
}

// withRetry runs fn up to 3 times with exponential backoff.
func withRetry(fn func() error) error {
	delay := time.Second
	var err error
	for range 3 {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(delay)
		delay *= 2
	}
	return err
}
