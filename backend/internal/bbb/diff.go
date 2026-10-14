package bbb

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// EventKind is what happened to an item between two runs.
type EventKind string

// The event kinds, matching the event_kind enum in the database.
const (
	EventNew     EventKind = "new"
	EventEdit    EventKind = "edit"
	EventMoved   EventKind = "moved"
	EventRemoved EventKind = "removed"
)

// RepostWindow is how long after removal a new item with the same title is
// treated as a repost of the removed one.
const RepostWindow = 14 * 24 * time.Hour

// Known is the stored state of an item the differ compares against.
type Known struct {
	PanelID     int64
	Division    Division
	Position    int
	VersionID   int64 // current version
	ContentHash string
	Title       string
	Subheader   string
	BodyText    string
	SignupLinks []string
	RemovedAt   *time.Time
}

// State is everything the differ needs from the database.
type State struct {
	Items map[int64]Known
	// LastGoodCount is the item count of the last non-suspect run, or 0 if
	// there has been none.
	LastGoodCount int
}

// Upsert is the new state of one item seen on the page. NewVersion is true
// when Version differs from the stored current version and must be inserted.
type Upsert struct {
	Item       Item
	Version    Version
	NewVersion bool
}

// Event is one change. The store fills in version ids: for new and edit the
// to-version is the one inserted this run, for moved it is the item's current
// version, and removed has only a from-version.
type Event struct {
	PanelID       int64
	Kind          EventKind
	FromVersionID int64 // 0 when there is none
	RepostOf      int64 // panel id of the removed item this reposts, or 0
	BecameSignup  bool
	WordsAdded    int
	WordsRemoved  int
}

// Plan is what one run should write.
type Plan struct {
	// Suspect runs write nothing but the scrape_runs row.
	Suspect        bool
	SuspectReasons []string
	// Baseline is set on the very first run, when nothing is stored yet:
	// items and versions are written but no events, so the first deploy
	// doesn't announce every item on the page as new.
	Baseline bool
	Upserts  []Upsert
	Removals []int64
	Events   []Event
}

// Diff compares a parsed page with the stored state and plans the writes.
// now is the run time, in the school's time zone so deadlines infer the
// right year.
func Diff(state State, page *Page, sd SignupDetector, now time.Time) Plan {
	if reasons := suspectReasons(state, page); len(reasons) > 0 {
		return Plan{Suspect: true, SuspectReasons: reasons}
	}

	plan := Plan{Baseline: len(state.Items) == 0}
	today := DateOf(now)
	seen := make(map[int64]bool, len(page.Items))

	for _, it := range page.Items {
		seen[it.PanelID] = true
		v := Normalize(it, sd, today)
		known, ok := state.Items[it.PanelID]
		if !ok {
			plan.Upserts = append(plan.Upserts, Upsert{Item: it, Version: v, NewVersion: true})
			plan.Events = append(plan.Events, Event{
				PanelID:  it.PanelID,
				Kind:     EventNew,
				RepostOf: repostOf(state, v.Title, it.PanelID, now),
			})
			continue
		}

		changed := known.ContentHash != v.ContentHash
		plan.Upserts = append(plan.Upserts, Upsert{Item: it, Version: v, NewVersion: changed})
		if changed {
			// Reappearing after removal with different content is an edit
			// against the last version; with the same content it just
			// comes back without an event.
			removed, added := WordDelta(fullText(known.Title, known.Subheader, known.BodyText),
				fullText(v.Title, v.Subheader, v.BodyText))
			plan.Events = append(plan.Events, Event{
				PanelID:       it.PanelID,
				Kind:          EventEdit,
				FromVersionID: known.VersionID,
				BecameSignup:  addsLink(known.SignupLinks, v.SignupLinks),
				WordsAdded:    added,
				WordsRemoved:  removed,
			})
		}
		if known.Division != it.Division {
			plan.Events = append(plan.Events, Event{
				PanelID:       it.PanelID,
				Kind:          EventMoved,
				FromVersionID: known.VersionID,
			})
		}
	}

	for id, known := range state.Items {
		if known.RemovedAt == nil && !seen[id] {
			plan.Removals = append(plan.Removals, id)
		}
	}
	// Sorted so runs are reproducible despite the map iteration.
	slices.Sort(plan.Removals)
	for _, id := range plan.Removals {
		plan.Events = append(plan.Events, Event{
			PanelID:       id,
			Kind:          EventRemoved,
			FromVersionID: state.Items[id].VersionID,
		})
	}

	if plan.Baseline {
		plan.Events = nil
	}
	return plan
}

func suspectReasons(state State, page *Page) []string {
	var reasons []string
	if len(page.Items) == 0 {
		reasons = append(reasons, "no items parsed")
	}
	for _, d := range page.MissingDivisions {
		reasons = append(reasons, fmt.Sprintf("division %s missing", d))
	}
	if n := len(page.Items); n > 0 && n*2 < state.LastGoodCount {
		reasons = append(reasons, fmt.Sprintf("item count fell from %d to %d", state.LastGoodCount, n))
	}
	return reasons
}

// repostOf returns the panel id of an item removed within RepostWindow whose
// title matches, or 0. Ties go to the most recently removed.
func repostOf(state State, title string, self int64, now time.Time) int64 {
	key := titleKey(title)
	var best int64
	var bestAt time.Time
	for id, k := range state.Items {
		if id == self || k.RemovedAt == nil || now.Sub(*k.RemovedAt) > RepostWindow {
			continue
		}
		if titleKey(k.Title) != key {
			continue
		}
		if best == 0 || k.RemovedAt.After(bestAt) || (k.RemovedAt.Equal(bestAt) && id > best) {
			best, bestAt = id, *k.RemovedAt
		}
	}
	return best
}

func titleKey(s string) string {
	return strings.ToLower(collapseLine(s))
}

func fullText(title, sub, body string) string {
	return title + "\n" + sub + "\n" + body
}

func addsLink(before, after []string) bool {
	for _, l := range after {
		if !slices.Contains(before, l) {
			return true
		}
	}
	return false
}
