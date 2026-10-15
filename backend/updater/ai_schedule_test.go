package updater

import (
	"testing"
	"time"
)

func scheduleTime(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAIScheduleMoscowMondayThursdayBoundaries(t *testing.T) {
	for _, tc := range []struct{ now, previous, next string }{
		{"2026-10-12T20:59:59+03:00", "2026-10-08T21:00:00+03:00", "2026-10-12T21:00:00+03:00"},
		{"2026-10-12T18:00:00Z", "2026-10-12T21:00:00+03:00", "2026-10-15T21:00:00+03:00"},
		{"2026-10-13T12:00:00+03:00", "2026-10-12T21:00:00+03:00", "2026-10-15T21:00:00+03:00"},
		{"2026-10-15T21:00:00+03:00", "2026-10-15T21:00:00+03:00", "2026-10-19T21:00:00+03:00"},
		{"2026-10-18T23:59:59+03:00", "2026-10-15T21:00:00+03:00", "2026-10-19T21:00:00+03:00"},
		{"2026-12-31T21:00:00+03:00", "2026-12-31T21:00:00+03:00", "2027-01-04T21:00:00+03:00"},
	} {
		previous, next := aiScheduleSlots(scheduleTime(t, tc.now))
		if !previous.Equal(scheduleTime(t, tc.previous)) || !next.Equal(scheduleTime(t, tc.next)) {
			t.Fatalf("%s: got %s / %s", tc.now, previous, next)
		}
	}
}

func TestAIScheduleNoOffSlotStartupCatchupAndRestartDeduplication(t *testing.T) {
	slot := scheduleTime(t, "2026-10-15T21:00:00+03:00")
	old := slot.Add(-7 * 24 * time.Hour)
	for _, last := range []*time.Time{nil, &old} {
		for _, now := range []time.Time{slot.Add(-time.Minute), slot.Add(30 * time.Minute), slot.Add(24 * time.Hour)} {
			if aiScheduledRunDue(now, last) {
				t.Fatalf("off-slot startup made a paid run: %s", now)
			}
		}
		for _, now := range []time.Time{slot, slot.Add(29 * time.Minute)} {
			if !aiScheduledRunDue(now, last) {
				t.Fatalf("scheduled/recovery slot missed: %s", now)
			}
		}
	}
	for _, last := range []time.Time{slot, slot.Add(time.Minute)} {
		if aiScheduledRunDue(slot.Add(2*time.Minute), &last) {
			t.Fatal("restart repeated a processed slot")
		}
	}
}
