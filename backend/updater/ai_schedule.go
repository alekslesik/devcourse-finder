package updater

import "time"

// AI collection runs on Monday and Thursday at 21:00 Moscow time. A short
// same-slot recovery window permits restarts without paid catch-up on other days.
const aiScheduleRecoveryWindow = 30 * time.Minute

func aiScheduleSlots(now time.Time) (previous, next time.Time) {
	location, _ := time.LoadLocation("Europe/Moscow")
	local := now.In(location)
	for day := -7; day <= 7; day++ {
		slot := time.Date(local.Year(), local.Month(), local.Day()+day, 21, 0, 0, 0, location)
		if slot.Weekday() != time.Monday && slot.Weekday() != time.Thursday {
			continue
		}
		if !slot.After(now) {
			previous = slot
		} else {
			next = slot
			break
		}
	}
	return previous, next
}

func aiScheduledRunDue(now time.Time, last *time.Time) bool {
	previous, _ := aiScheduleSlots(now)
	return !now.Before(previous) && now.Before(previous.Add(aiScheduleRecoveryWindow)) && (last == nil || last.Before(previous))
}
