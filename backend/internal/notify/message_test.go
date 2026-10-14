package notify

import (
	"testing"
	"time"

	"github.com/farrellm/bbb/internal/store"
)

func defaults() store.Preferences {
	return store.Preferences{
		Divisions: []string{"all", "lower", "middle", "upper"},
		NotifyNew: true, NotifyEdit: true, MinEditWords: 1,
	}
}

func TestMatches(t *testing.T) {
	lower := func(p store.Preferences) store.Preferences {
		p.Divisions = []string{"lower"}
		return p
	}
	tests := []struct {
		name  string
		prefs store.Preferences
		ev    store.PendingEvent
		want  bool
	}{
		{"new", defaults(), store.PendingEvent{Kind: "new", Division: "upper"}, true},
		{
			"new off", func() store.Preferences { p := defaults(); p.NotifyNew = false; return p }(),
			store.PendingEvent{Kind: "new", Division: "upper"},
			false,
		},
		{"edit", defaults(), store.PendingEvent{Kind: "edit", Division: "all", WordsAdded: 1}, true},
		{
			"edit too small", func() store.Preferences { p := defaults(); p.MinEditWords = 5; return p }(),
			store.PendingEvent{Kind: "edit", Division: "all", WordsAdded: 2, WordsRemoved: 2},
			false,
		},
		{"punctuation-only edit", defaults(), store.PendingEvent{Kind: "edit", Division: "all"}, false},
		{
			"small edit adding a signup link", func() store.Preferences { p := defaults(); p.MinEditWords = 5; return p }(),
			store.PendingEvent{Kind: "edit", Division: "all", WordsAdded: 1, BecameSignup: true},
			true,
		},
		{"removed off by default", defaults(), store.PendingEvent{Kind: "removed", Division: "all"}, false},
		{
			"removed on", func() store.Preferences { p := defaults(); p.NotifyRemoved = true; return p }(),
			store.PendingEvent{Kind: "removed", Division: "all"},
			true,
		},
		{"moved never", defaults(), store.PendingEvent{Kind: "moved", Division: "all"}, false},
		{"other division", lower(defaults()), store.PendingEvent{Kind: "new", Division: "upper"}, false},
		{
			"other division signup", lower(defaults()),
			store.PendingEvent{Kind: "new", Division: "upper", IsSignup: true},
			false,
		},
		{
			"other division signup, any division on",
			func() store.Preferences { p := lower(defaults()); p.SignupsAnyDivision = true; return p }(),
			store.PendingEvent{Kind: "new", Division: "upper", IsSignup: true},
			true,
		},
		{
			"signups only", func() store.Preferences { p := defaults(); p.SignupsOnly = true; return p }(),
			store.PendingEvent{Kind: "new", Division: "lower"},
			false,
		},
		{
			"signups only, signup", func() store.Preferences { p := defaults(); p.SignupsOnly = true; return p }(),
			store.PendingEvent{Kind: "new", Division: "lower", IsSignup: true},
			true,
		},
	}
	for _, tt := range tests {
		if got := Matches(tt.prefs, tt.ev); got != tt.want {
			t.Errorf("%s: Matches = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestSingle(t *testing.T) {
	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		ev   store.PendingEvent
		want Payload
	}{
		{
			store.PendingEvent{
				PanelID: 7, Kind: "new", Division: "lower", Title: "Lunchroom Volunteer Program",
				IsSignup: true, Deadline: &due,
			},
			Payload{
				Title: "New in Lower School", Body: "Lunchroom Volunteer Program. Sign-up, due Oct 20",
				URL: "/items/7", Tag: "item-7",
			},
		},
		{
			store.PendingEvent{PanelID: 7, Kind: "new", Division: "all", Title: "Join Us!", IsSignup: true},
			Payload{Title: "New in All School", Body: "Join Us! Sign-up", URL: "/items/7", Tag: "item-7"},
		},
		{
			store.PendingEvent{
				PanelID: 8, Kind: "edit", Division: "all", Title: "Friday Night Lights",
				FromVersionID: ptr[int64](3), ToVersionID: ptr[int64](9), WordsAdded: 12, WordsRemoved: 4,
			},
			Payload{
				Title: "Edited in All School", Body: "Friday Night Lights: 12 words added, 4 removed",
				URL: "/items/8/diff?from=3&to=9", Tag: "item-8",
			},
		},
		{
			store.PendingEvent{
				PanelID: 8, Kind: "edit", Division: "upper", Title: "Gala",
				FromVersionID: ptr[int64](3), ToVersionID: ptr[int64](9), WordsAdded: 1, BecameSignup: true,
			},
			Payload{
				Title: "Edited in Upper School", Body: "Gala: sign-up link added",
				URL: "/items/8/diff?from=3&to=9", Tag: "item-8",
			},
		},
		{
			store.PendingEvent{PanelID: 9, Kind: "edit", Division: "middle", Title: "Picnic", WordsRemoved: 1},
			Payload{Title: "Edited in Middle School", Body: "Picnic: 1 word removed", URL: "/items/9", Tag: "item-9"},
		},
		{
			store.PendingEvent{PanelID: 9, Kind: "removed", Division: "middle", Title: "Picnic"},
			Payload{Title: "Removed from Middle School", Body: "Picnic", URL: "/items/9", Tag: "item-9"},
		},
	}
	for _, tt := range tests {
		if got := single(tt.ev); got != tt.want {
			t.Errorf("single(%+v)\n got %+v\nwant %+v", tt.ev, got, tt.want)
		}
	}
}

func TestPlanBatchesBigRuns(t *testing.T) {
	var evs []store.PendingEvent
	add := func(run int64, kind string, n int) {
		for range n {
			evs = append(evs, store.PendingEvent{
				ID: int64(len(evs) + 1), RunID: run, PanelID: int64(len(evs) + 100),
				Kind: kind, Division: "all", Title: "x", WordsAdded: 1,
			})
		}
	}
	add(1, "new", 2) // run 1: 2 events, sent singly
	add(2, "new", 3) // run 2: 5 events, one summary
	add(2, "edit", 1)
	add(2, "removed", 1)

	msgs := plan(evs)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(msgs), msgs)
	}
	sum := msgs[2]
	want := Payload{Title: "5 BBB updates", Body: "3 new, 1 edited, 1 removed", URL: "/", Tag: "summary"}
	if sum.payload != want || len(sum.events) != 5 {
		t.Errorf("summary = %+v covering %v", sum.payload, sum.events)
	}
}

func TestAllowedEndpoint(t *testing.T) {
	for ep, want := range map[string]bool{
		"https://web.push.apple.com/3/device/abc":         true,
		"https://fcm.googleapis.com/fcm/send/abc":         true,
		"https://updates.push.services.mozilla.com/wpush": true,
		"https://wns2-par02p.notify.windows.com/w/?token": true,
		"http://web.push.apple.com/3/device/abc":          false,
		"https://web.push.apple.com:8443/x":               false,
		"https://evilpush.apple.com.example/x":            false,
		"https://127.0.0.1/x":                             false,
		"https://user@web.push.apple.com/x":               false,
		"not a url":                                       false,
	} {
		if got := AllowedEndpoint(ep); got != want {
			t.Errorf("AllowedEndpoint(%q) = %v", ep, got)
		}
	}
}
