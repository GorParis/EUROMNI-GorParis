package main

import (
	"testing"
	"time"
)

func TestDurationFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("EUROMNI_TEST_DURATION", "")
		got, err := durationFromEnv("EUROMNI_TEST_DURATION", 24*time.Hour)
		if err != nil || got != 24*time.Hour {
			t.Fatalf("durationFromEnv() = %s, error = %v; want 24h", got, err)
		}
	})

	t.Run("configured", func(t *testing.T) {
		t.Setenv("EUROMNI_TEST_DURATION", "36h")
		got, err := durationFromEnv("EUROMNI_TEST_DURATION", time.Hour)
		if err != nil || got != 36*time.Hour {
			t.Fatalf("durationFromEnv() = %s, error = %v; want 36h", got, err)
		}
	})

	for _, value := range []string{"0s", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("EUROMNI_TEST_DURATION", value)
			if _, err := durationFromEnv("EUROMNI_TEST_DURATION", time.Hour); err == nil {
				t.Fatalf("durationFromEnv() accepted %q", value)
			}
		})
	}
}
