package main

import (
	"testing"
	"time"
)

func TestMailboxActivityLevel(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		lastPolled time.Time
		want       string
	}{
		{"never polled", time.Time{}, activityLevelOffline},
		{"just polled", now.Add(-30 * time.Second), activityLevelOnline},
		{"two minutes", now.Add(-2 * time.Minute), activityLevelOnline},
		{"five minutes", now.Add(-5 * time.Minute), activityLevelRecent},
		{"fifteen minutes", now.Add(-15 * time.Minute), activityLevelRecent},
		{"one hour", now.Add(-time.Hour), activityLevelIdle},
		{"twenty four hours", now.Add(-24 * time.Hour), activityLevelIdle},
		{"two days", now.Add(-48 * time.Hour), activityLevelOffline},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mailboxActivityLevel(tc.lastPolled, now); got != tc.want {
				t.Fatalf("mailboxActivityLevel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMailboxActivityCounts(t *testing.T) {
	online, recent := mailboxActivityCounts([]string{
		activityLevelOnline,
		activityLevelRecent,
		activityLevelIdle,
		activityLevelOffline,
		activityLevelOnline,
	})
	if online != 2 || recent != 1 {
		t.Fatalf("counts online=%d recent=%d, want 2 and 1", online, recent)
	}
}
