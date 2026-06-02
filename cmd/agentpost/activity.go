package main

import "time"

const (
	activityOnlineWindow = 2 * time.Minute
	activityRecentWindow = 15 * time.Minute
	activityIdleWindow   = 24 * time.Hour
)

const (
	activityLevelOnline  = "online"
	activityLevelRecent  = "recent"
	activityLevelIdle    = "idle"
	activityLevelOffline = "offline"
)

func mailboxActivityLevel(lastPolled time.Time, now time.Time) string {
	if lastPolled.IsZero() {
		return activityLevelOffline
	}
	ago := now.Sub(lastPolled)
	switch {
	case ago < 0:
		return activityLevelOnline
	case ago <= activityOnlineWindow:
		return activityLevelOnline
	case ago <= activityRecentWindow:
		return activityLevelRecent
	case ago <= activityIdleWindow:
		return activityLevelIdle
	default:
		return activityLevelOffline
	}
}

func mailboxActivityCounts(levels []string) (online, recent int) {
	for _, level := range levels {
		switch level {
		case activityLevelOnline:
			online++
		case activityLevelRecent:
			recent++
		}
	}
	return online, recent
}
