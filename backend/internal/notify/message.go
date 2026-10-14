package notify

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"

	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/store"
)

// batchOver is how many matching events one run can produce for a user
// before they arrive as a single summary instead.
const batchOver = 3

// Matches reports whether an event notifies a user with these preferences.
func Matches(p store.Preferences, e store.PendingEvent) bool {
	switch bbb.EventKind(e.Kind) {
	case bbb.EventNew:
		if !p.NotifyNew {
			return false
		}
	case bbb.EventEdit:
		if !p.NotifyEdit {
			return false
		}
		// Gaining a signup link is news however few words changed.
		if !e.BecameSignup && e.WordsAdded+e.WordsRemoved < p.MinEditWords {
			return false
		}
	case bbb.EventRemoved:
		if !p.NotifyRemoved {
			return false
		}
	case bbb.EventMoved:
		return false
	}
	signup := e.IsSignup || e.BecameSignup
	if p.SignupsOnly && !signup {
		return false
	}
	return slices.Contains(p.Divisions, e.Division) || (signup && p.SignupsAnyDivision)
}

// message is one push and the events it covers.
type message struct {
	events  []int64
	payload Payload
}

// plan turns one user's matching events into pushes: one per event, except
// that a run with more than batchOver of them becomes one summary.
func plan(evs []store.PendingEvent) []message {
	var out []message
	for run := range chunkByRun(evs) {
		if len(run) > batchOver {
			ids := make([]int64, len(run))
			for i, e := range run {
				ids[i] = e.ID
			}
			out = append(out, message{events: ids, payload: summary(run)})
			continue
		}
		for _, e := range run {
			out = append(out, message{events: []int64{e.ID}, payload: single(e)})
		}
	}
	return out
}

// chunkByRun yields consecutive events that share a run. Events come
// ordered by id, and a run's events are written together.
func chunkByRun(evs []store.PendingEvent) iter.Seq[[]store.PendingEvent] {
	return func(yield func([]store.PendingEvent) bool) {
		for len(evs) > 0 {
			n := 1
			for n < len(evs) && evs[n].RunID == evs[0].RunID {
				n++
			}
			if !yield(evs[:n]) {
				return
			}
			evs = evs[n:]
		}
	}
}

// released is the one push for what was held during a user's quiet hours:
// the event itself if there was one, otherwise a summary.
func released(evs []store.PendingEvent) message {
	if len(evs) == 1 {
		return message{events: ids(evs), payload: single(evs[0])}
	}
	return message{events: ids(evs), payload: summary(evs)}
}

// Scraper alerts share a tag, so the all-clear replaces the alarm.
const scraperTag = "scraper"

// ScraperStuck is the admin alert after runs bad polls in a row.
func ScraperStuck(runs int) Payload {
	return Payload{
		Title: "BBB Watch can't read the page",
		Body: fmt.Sprintf("The last %d checks of pingry.org failed or looked wrong. "+
			"Nothing will be announced until it reads the page again.", runs),
		URL: "/settings",
		Tag: scraperTag,
	}
}

// ScraperRecovered is the admin all-clear after ScraperStuck.
func ScraperRecovered() Payload {
	return Payload{
		Title: "BBB Watch is reading the page again",
		Body:  "Checks of pingry.org are working again.",
		URL:   "/",
		Tag:   scraperTag,
	}
}

var divisionNames = map[string]string{
	"all":    "All School",
	"lower":  "Lower School",
	"middle": "Middle School",
	"upper":  "Upper School",
}

// single describes one event: "New in Lower School" / "Lunchroom
// Volunteer Program. Sign-up, due Oct 20".
func single(e store.PendingEvent) Payload {
	id := strconv.FormatInt(e.PanelID, 10)
	div := divisionNames[e.Division]
	p := Payload{Body: e.Title, URL: "/items/" + id, Tag: "item-" + id}
	switch bbb.EventKind(e.Kind) {
	case bbb.EventNew:
		p.Title = "New in " + div
		p.Body += signupNote(e)
	case bbb.EventEdit:
		p.Title = "Edited in " + div
		p.Body += ": " + change(e)
		if e.FromVersionID != nil && e.ToVersionID != nil {
			p.URL = fmt.Sprintf("/items/%s/diff?from=%d&to=%d", id, *e.FromVersionID, *e.ToVersionID)
		}
	case bbb.EventMoved:
		p.Title = "Moved to " + div
	case bbb.EventRemoved:
		p.Title = "Removed from " + div
	}
	return p
}

// signupNote is ". Sign-up, due Oct 20" for a signup item, or "".
func signupNote(e store.PendingEvent) string {
	if !e.IsSignup {
		return ""
	}
	note := "Sign-up"
	if e.Deadline != nil {
		note += ", due " + e.Deadline.Format("Jan 2")
	}
	if strings.HasSuffix(e.Title, ".") || strings.HasSuffix(e.Title, "!") || strings.HasSuffix(e.Title, "?") {
		return " " + note
	}
	return ". " + note
}

// change summarizes an edit as the app does: "12 words added, 4 removed".
func change(e store.PendingEvent) string {
	if e.BecameSignup {
		return "sign-up link added"
	}
	a, r := e.WordsAdded, e.WordsRemoved
	switch {
	case a > 0 && r > 0:
		return fmt.Sprintf("%s added, %d removed", words(a), r)
	case a > 0:
		return words(a) + " added"
	case r > 0:
		return words(r) + " removed"
	default:
		return "punctuation or formatting changed"
	}
}

func words(n int) string {
	if n == 1 {
		return "1 word"
	}
	return strconv.Itoa(n) + " words"
}

// summary describes a batch: "5 BBB updates" / "3 new, 2 edited".
func summary(evs []store.PendingEvent) Payload {
	counts := map[bbb.EventKind]int{}
	for _, e := range evs {
		counts[bbb.EventKind(e.Kind)]++
	}
	var parts []string
	for _, k := range []struct {
		kind bbb.EventKind
		verb string
	}{{bbb.EventNew, "new"}, {bbb.EventEdit, "edited"}, {bbb.EventRemoved, "removed"}} {
		if n := counts[k.kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k.verb))
		}
	}
	return Payload{
		Title: fmt.Sprintf("%d BBB updates", len(evs)),
		Body:  strings.Join(parts, ", "),
		URL:   "/",
		Tag:   "summary",
	}
}
