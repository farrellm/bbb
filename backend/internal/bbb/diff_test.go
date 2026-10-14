package bbb

import (
	"os"
	"strings"
	"testing"
	"time"
)

var (
	runTime = time.Date(2026, time.October, 14, 20, 5, 0, 0, time.UTC)
	sd      = SignupDetector{Hosts: DefaultSignupHosts}
)

func sampleHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/bbb-sample.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func parseString(t *testing.T, s string) *Page {
	t.Helper()
	page, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// stateOf builds the stored state that would exist after a clean run of s.
func stateOf(t *testing.T, s string) State {
	t.Helper()
	page := parseString(t, s)
	st := State{Items: map[int64]Known{}, LastGoodCount: len(page.Items)}
	for _, it := range page.Items {
		v := Normalize(it, sd, DateOf(runTime))
		st.Items[it.PanelID] = Known{
			PanelID: it.PanelID, Division: it.Division, Position: it.Position,
			VersionID: it.PanelID * 10, ContentHash: v.ContentHash,
			Title: v.Title, Subheader: v.Subheader, BodyText: v.BodyText,
			SignupLinks: v.SignupLinks,
		}
	}
	return st
}

// panelBounds returns the byte range of the <section> for a panel.
func panelBounds(t *testing.T, s string, id string) (int, int) {
	t.Helper()
	at := strings.Index(s, `id="fsEl_`+id+`"`)
	if at < 0 {
		t.Fatalf("panel %s not in sample", id)
	}
	start := strings.LastIndex(s[:at], "<section")
	end := at + strings.Index(s[at:], "</section>") + len("</section>")
	return start, end
}

// editPanel applies a string replacement inside one panel only.
func editPanel(t *testing.T, s, id, old, repl string) string {
	t.Helper()
	start, end := panelBounds(t, s, id)
	section := s[start:end]
	if !strings.Contains(section, old) {
		t.Fatalf("%q not in panel %s", old, id)
	}
	return s[:start] + strings.Replace(section, old, repl, 1) + s[end:]
}

func TestDiff(t *testing.T) {
	base := sampleHTML(t)

	tests := []struct {
		name   string
		page   func(string) string
		state  func(*State)
		events []Event
	}{
		{
			name:   "unchanged page",
			page:   func(s string) string { return s },
			events: nil,
		},
		{
			name: "added panel is new",
			page: func(s string) string {
				start, end := panelBounds(t, s, "67589")
				clone := strings.ReplaceAll(s[start:end], "67589", "99999")
				return s[:end] + clone + s[end:]
			},
			events: []Event{{PanelID: 99999, Kind: EventNew}},
		},
		{
			name: "body edit counts words",
			page: func(s string) string {
				return editPanel(t, s, "67589", "November 1, 2026", "November 15, 2026")
			},
			events: []Event{{PanelID: 67589, Kind: EventEdit, FromVersionID: 675890, WordsAdded: 1, WordsRemoved: 1}},
		},
		{
			name: "nbsp-only change is not an edit",
			page: func(s string) string {
				return editPanel(t, s, "67589", "are open.", "are&nbsp;open.&nbsp;")
			},
			events: nil,
		},
		{
			name: "markup-only change is not an edit",
			page: func(s string) string {
				return editPanel(t, s, "67589", "are open.", "<span>are open.</span>")
			},
			events: nil,
		},
		{
			name: "removed panel",
			page: func(s string) string {
				start, end := panelBounds(t, s, "67589")
				return s[:start] + s[end:]
			},
			events: []Event{{PanelID: 67589, Kind: EventRemoved, FromVersionID: 675890}},
		},
		{
			name:   "moved panel",
			page:   func(s string) string { return s },
			state:  func(st *State) { k := st.Items[67589]; k.Division = DivisionLower; st.Items[67589] = k },
			events: []Event{{PanelID: 67589, Kind: EventMoved, FromVersionID: 675890}},
		},
		{
			name: "edit that adds a signup link",
			page: func(s string) string {
				return editPanel(t, s, "67589", "are open.", `are open. <a href="https://www.signupgenius.com/go/new">Sign up</a>`)
			},
			// "Sign up" plus the six words of the URL kept in the text.
			events: []Event{{PanelID: 67589, Kind: EventEdit, FromVersionID: 675890, BecameSignup: true, WordsAdded: 8}},
		},
		{
			name: "reappearing with the same content has no event",
			page: func(s string) string { return s },
			state: func(st *State) {
				k := st.Items[67589]
				removed := runTime.Add(-time.Hour)
				k.RemovedAt = &removed
				st.Items[67589] = k
			},
			events: nil,
		},
		{
			name: "reappearing with new content is an edit",
			page: func(s string) string {
				return editPanel(t, s, "67589", "November 1, 2026", "November 15, 2026")
			},
			state: func(st *State) {
				k := st.Items[67589]
				removed := runTime.Add(-time.Hour)
				k.RemovedAt = &removed
				st.Items[67589] = k
			},
			events: []Event{{PanelID: 67589, Kind: EventEdit, FromVersionID: 675890, WordsAdded: 1, WordsRemoved: 1}},
		},
		{
			name: "repost of a recently removed item",
			page: func(s string) string {
				start, end := panelBounds(t, s, "67589")
				return s[:start] + strings.ReplaceAll(s[start:end], "67589", "99999") + s[end:]
			},
			state: func(st *State) {
				k := st.Items[67589]
				removed := runTime.Add(-3 * 24 * time.Hour)
				k.RemovedAt = &removed
				st.Items[67589] = k
			},
			events: []Event{{PanelID: 99999, Kind: EventNew, RepostOf: 67589}},
		},
		{
			name: "no repost after the window",
			page: func(s string) string {
				start, end := panelBounds(t, s, "67589")
				return s[:start] + strings.ReplaceAll(s[start:end], "67589", "99999") + s[end:]
			},
			state: func(st *State) {
				k := st.Items[67589]
				removed := runTime.Add(-15 * 24 * time.Hour)
				k.RemovedAt = &removed
				st.Items[67589] = k
			},
			events: []Event{{PanelID: 99999, Kind: EventNew}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := stateOf(t, base)
			if tt.state != nil {
				tt.state(&st)
			}
			plan := Diff(st, parseString(t, tt.page(base)), sd, runTime)
			if plan.Suspect || plan.Baseline {
				t.Fatalf("suspect=%v baseline=%v %v", plan.Suspect, plan.Baseline, plan.SuspectReasons)
			}
			if !eventsEqual(plan.Events, tt.events) {
				t.Errorf("events\n got %+v\nwant %+v", plan.Events, tt.events)
			}
			for _, u := range plan.Upserts {
				wantNew := false
				for _, e := range tt.events {
					if e.PanelID == u.Item.PanelID && (e.Kind == EventNew || e.Kind == EventEdit) {
						wantNew = true
					}
				}
				if u.NewVersion != wantNew {
					t.Errorf("panel %d NewVersion = %v, want %v", u.Item.PanelID, u.NewVersion, wantNew)
				}
			}
		})
	}
}

func eventsEqual(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiffSuspect(t *testing.T) {
	base := sampleHTML(t)
	st := stateOf(t, base)

	tests := []struct {
		name  string
		page  string
		state func(*State)
	}{
		{name: "empty page", page: ""},
		{name: "error page", page: "<html><body><h1>Service unavailable</h1></body></html>"},
		{name: "truncated page", page: base[:strings.Index(base, `id="middleschool"`)]},
		{name: "item count halved", page: base, state: func(st *State) { st.LastGoodCount = 100 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := st
			if tt.state != nil {
				tt.state(&s)
			}
			plan := Diff(s, parseString(t, tt.page), sd, runTime)
			if !plan.Suspect {
				t.Fatal("not suspect")
			}
			if len(plan.SuspectReasons) == 0 {
				t.Error("no reasons")
			}
			if len(plan.Events)+len(plan.Upserts)+len(plan.Removals) > 0 {
				t.Errorf("suspect plan writes: %+v", plan)
			}
		})
	}
}

func TestDiffBaseline(t *testing.T) {
	plan := Diff(State{}, parseString(t, sampleHTML(t)), sd, runTime)
	if !plan.Baseline || plan.Suspect {
		t.Fatalf("baseline=%v suspect=%v", plan.Baseline, plan.Suspect)
	}
	if len(plan.Events) != 0 {
		t.Errorf("baseline emitted %d events", len(plan.Events))
	}
	if len(plan.Upserts) != 41 {
		t.Errorf("%d upserts, want 41", len(plan.Upserts))
	}
	for _, u := range plan.Upserts {
		if !u.NewVersion {
			t.Errorf("panel %d: NewVersion false on baseline", u.Item.PanelID)
		}
	}
}
