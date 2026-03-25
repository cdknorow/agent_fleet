package background

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// NextFireTime computes the next fire time for a 5-field cron expression
// (minute hour dom month dow) after the given time, evaluated in the
// specified timezone. An empty tz defaults to UTC.
func NextFireTime(cronExpr, tz string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	loc := time.UTC
	if tz != "" {
		loc, err = time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
	}
	afterLocal := after.In(loc)
	next := sched.Next(afterLocal)
	return next.UTC(), nil
}
