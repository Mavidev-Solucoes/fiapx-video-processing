package store

import (
	"testing"
	"time"
)

func TestEmailBackoffIsBounded(t *testing.T) {
	cases := []struct {
		attempt int
		delay time.Duration
	}{
		{1, 5*time.Second}, {2,10*time.Second},
		{3,20*time.Second}, {6,160*time.Second}, {9,160*time.Second},
	}
	for _, tc := range cases {
		if got := retryEmailDelay(tc.attempt); got != tc.delay {
			t.Errorf("attempt %d => %s, expected %s",tc.attempt,got,tc.delay)
		}
	}
}
