package notify

import (
	"time"

	"github.com/farrellm/bbb/internal/store"
)

// ClockLayout is how quiet hours are written: "21:30".
const ClockLayout = "15:04"

// quiet reports whether t (in the school's time zone) falls inside the
// preferences' quiet hours. Hours that cross midnight, such as 21:30 to
// 07:00, are quiet from the start through to the end the next morning.
func quiet(p store.Preferences, t time.Time) bool {
	if p.QuietStart == nil || p.QuietEnd == nil {
		return false
	}
	start, err1 := time.Parse(ClockLayout, *p.QuietStart)
	end, err2 := time.Parse(ClockLayout, *p.QuietEnd)
	if err1 != nil || err2 != nil {
		return false
	}
	s := start.Hour()*60 + start.Minute()
	e := end.Hour()*60 + end.Minute()
	m := t.Hour()*60 + t.Minute()
	switch {
	case s < e:
		return s <= m && m < e
	case s > e:
		return m >= s || m < e
	default: // empty; the API and the database refuse these
		return false
	}
}
