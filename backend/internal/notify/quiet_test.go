package notify

import (
	"testing"
	"time"
)

func TestQuiet(t *testing.T) {
	at := func(clock string) time.Time {
		c, err := time.Parse(ClockLayout, clock)
		if err != nil {
			t.Fatal(err)
		}
		return time.Date(2026, time.October, 14, c.Hour(), c.Minute(), 30, 0, time.UTC)
	}
	tests := []struct {
		start, end, now string
		want            bool
	}{
		{"", "", "03:00", false},
		// Crossing midnight.
		{"21:30", "07:00", "21:29", false},
		{"21:30", "07:00", "21:30", true},
		{"21:30", "07:00", "00:00", true},
		{"21:30", "07:00", "06:59", true},
		{"21:30", "07:00", "07:00", false},
		{"21:30", "07:00", "12:00", false},
		// Within a day.
		{"13:00", "15:00", "12:59", false},
		{"13:00", "15:00", "13:00", true},
		{"13:00", "15:00", "15:00", false},
		// Empty, which the API refuses: never quiet.
		{"13:00", "13:00", "13:00", false},
	}
	for _, tt := range tests {
		p := defaults()
		if tt.start != "" {
			p.QuietStart, p.QuietEnd = &tt.start, &tt.end
		}
		if got := quiet(p, at(tt.now)); got != tt.want {
			t.Errorf("quiet(%s-%s, %s) = %v, want %v", tt.start, tt.end, tt.now, got, tt.want)
		}
	}
}
