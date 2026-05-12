package core

import (
	"os"
	"strconv"
	"time"

	"github.com/quickowl/knify/apps/session-monitor/internal/types"
)

func FirstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func EnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func IntEnv(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func BoolEnv(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func DurationEnv(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func LookbackEnv() time.Duration {
	if value := os.Getenv("SESSION_MONITOR_LOOKBACK_HOURS"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed > 0 {
			return time.Duration(parsed * float64(time.Hour))
		}
	}
	return types.DefaultLookback
}
