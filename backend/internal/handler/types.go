package handler

import (
	"time"

	"github.com/farrellm/bbb/internal/store"
)

// The JSON shapes below are the API contract; backend/api/openapi.yaml
// describes the same shapes and the frontend's types are generated from it.
// Change both together.

type userJSON struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	PictureURL string `json:"pictureUrl"`
	IsAdmin    bool   `json:"isAdmin"`
}

func toUser(u store.User) userJSON {
	return userJSON{ID: u.ID, Email: u.Email, Name: u.Name, PictureURL: u.PictureURL, IsAdmin: u.IsAdmin}
}

type eventJSON struct {
	ID            int64     `json:"id"`
	PanelID       int64     `json:"panelId"`
	Kind          string    `json:"kind"`
	CreatedAt     time.Time `json:"createdAt"`
	FromVersionID *int64    `json:"fromVersionId"`
	ToVersionID   *int64    `json:"toVersionId"`
	RepostOf      *int64    `json:"repostOf"`
	BecameSignup  bool      `json:"becameSignup"`
	WordsAdded    int       `json:"wordsAdded"`
	WordsRemoved  int       `json:"wordsRemoved"`
	Unread        bool      `json:"unread"`
}

func toEvent(e store.Event) eventJSON {
	return eventJSON{
		ID: e.ID, PanelID: e.PanelID, Kind: e.Kind, CreatedAt: e.CreatedAt,
		FromVersionID: e.FromVersionID, ToVersionID: e.ToVersionID, RepostOf: e.RepostOf,
		BecameSignup: e.BecameSignup, WordsAdded: e.WordsAdded, WordsRemoved: e.WordsRemoved,
		Unread: e.Unread,
	}
}

type timelineEventJSON struct {
	eventJSON
	Division string `json:"division"`
	Title    string `json:"title"`
	IsSignup bool   `json:"isSignup"`
}

func toTimelineEvents(evs []store.TimelineEvent) []timelineEventJSON {
	out := make([]timelineEventJSON, len(evs))
	for i, e := range evs {
		out[i] = timelineEventJSON{eventJSON: toEvent(e.Event), Division: e.Division, Title: e.Title, IsSignup: e.IsSignup}
	}
	return out
}

type signupLinkJSON struct {
	URL   string `json:"url"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

type itemJSON struct {
	PanelID      int64            `json:"panelId"`
	Division     string           `json:"division"`
	Position     int              `json:"position"`
	VersionID    int64            `json:"versionId"`
	VersionCount int              `json:"versionCount"`
	Title        string           `json:"title"`
	Subheader    string           `json:"subheader"`
	BodyHTML     string           `json:"bodyHtml"`
	IsSignup     bool             `json:"isSignup"`
	SignupLinks  []signupLinkJSON `json:"signupLinks"`
	Deadline     *string          `json:"deadline"`
	FirstSeenAt  time.Time        `json:"firstSeenAt"`
	LastSeenAt   time.Time        `json:"lastSeenAt"`
	RemovedAt    *time.Time       `json:"removedAt"`
	LatestEvent  *eventJSON       `json:"latestEvent"`
}

type versionMetaJSON struct {
	ID     int64     `json:"id"`
	SeenAt time.Time `json:"seenAt"`
	Title  string    `json:"title"`
}

type versionJSON struct {
	ID          int64            `json:"id"`
	PanelID     int64            `json:"panelId"`
	SeenAt      time.Time        `json:"seenAt"`
	Title       string           `json:"title"`
	Subheader   string           `json:"subheader"`
	BodyText    string           `json:"bodyText"`
	BodyHTML    string           `json:"bodyHtml"`
	SignupLinks []signupLinkJSON `json:"signupLinks"`
}

type itemListJSON struct {
	Items      []itemJSON `json:"items"`
	LastReadAt *time.Time `json:"lastReadAt"`
}

type itemDetailJSON struct {
	Item     itemJSON            `json:"item"`
	Versions []versionMetaJSON   `json:"versions"`
	Events   []timelineEventJSON `json:"events"`
}

type eventPageJSON struct {
	Events     []timelineEventJSON `json:"events"`
	NextBefore *int64              `json:"nextBefore"`
}

type statusJSON struct {
	CheckedAt *time.Time `json:"checkedAt"`
	OKAt      *time.Time `json:"okAt"`
	Failing   bool       `json:"failing"`
}

type inviteJSON struct {
	Code      string     `json:"code"`
	URL       string     `json:"url"`
	MaxUses   int        `json:"maxUses"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type scrapeRunJSON struct {
	ID         int64      `json:"id"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	HTTPStatus *int       `json:"httpStatus"`
	Unchanged  bool       `json:"unchanged"`
	ItemCount  *int       `json:"itemCount"`
	Suspect    bool       `json:"suspect"`
	Error      *string    `json:"error"`
}
