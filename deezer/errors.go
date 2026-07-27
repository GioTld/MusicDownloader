package deezer

import "fmt"

// ErrAuth is returned when the ARL cookie is invalid or expired.
type ErrAuth struct {
	Reason string
}

func (e *ErrAuth) Error() string {
	return fmt.Sprintf("auth: %s", e.Reason)
}

// ErrNotFound is returned when a requested Deezer resource does not exist.
type ErrNotFound struct {
	Type MediaType
	ID   string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s %q not found", e.Type, e.ID)
}

// ErrUnavailable is returned when a track exists but cannot be streamed,
// for example due to regional restrictions or a missing audio file.
type ErrUnavailable struct {
	TrackID string
	Reason  string
}

func (e *ErrUnavailable) Error() string {
	if e.TrackID != "" {
		return fmt.Sprintf("track %s unavailable: %s", e.TrackID, e.Reason)
	}
	return fmt.Sprintf("stream unavailable: %s", e.Reason)
}
