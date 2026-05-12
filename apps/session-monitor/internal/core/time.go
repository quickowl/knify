package core

import (
	"encoding/json"
	"strings"
	"time"
)

func ParseRecordTime(record map[string]any) time.Time {
	if got := ParseTime(StringValue(record["timestamp"])); !got.IsZero() {
		return got
	}
	if got := TimeFromMillis(record["timestamp"]); !got.IsZero() {
		return got
	}
	payload := MapValue(record["payload"])
	if payload != nil {
		if got := ParseTime(StringValue(payload["timestamp"])); !got.IsZero() {
			return got
		}
		if got := TimeFromSeconds(payload["started_at"]); !got.IsZero() {
			return got
		}
	}
	return time.Time{}
}

func ParseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if got, err := time.Parse(layout, raw); err == nil {
			return got.UTC()
		}
	}
	return time.Time{}
}

func TimeFromRFC3339(raw string) time.Time {
	return ParseTime(raw)
}

func TimeFromMillis(value any) time.Time {
	switch got := value.(type) {
	case float64:
		if got > 0 {
			return time.UnixMilli(int64(got)).UTC()
		}
	case json.Number:
		if n, err := got.Int64(); err == nil && n > 0 {
			return time.UnixMilli(n).UTC()
		}
	}
	return time.Time{}
}

func TimeFromSeconds(value any) time.Time {
	switch got := value.(type) {
	case float64:
		if got > 0 {
			sec := int64(got)
			nsec := int64((got - float64(sec)) * 1e9)
			return time.Unix(sec, nsec).UTC()
		}
	}
	return time.Time{}
}

func FormatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func Earliest(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}

func Latest(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.After(current) {
		return candidate
	}
	return current
}

func AbsDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
