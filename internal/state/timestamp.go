package state

import "time"

// TimeLayout is the DR-10 timestamp format: UTC, seconds, trailing Z.
const TimeLayout = "2006-01-02T15:04:05Z"

// FormatTime formats t in the DR-10 format.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime parses a DR-10 timestamp.
func ParseTime(value string) (time.Time, error) {
	return time.Parse(TimeLayout, value)
}
