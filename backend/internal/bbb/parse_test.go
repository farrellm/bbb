package bbb

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

// snapshotDate is when the raw page behind testdata/bbb-sample.html was saved.
var snapshotDate = Date{2026, time.October, 14}

func parseFile(t *testing.T, path string) *Page {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	page, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse(%s): %v", path, err)
	}
	return page
}

func TestParseSample(t *testing.T) {
	page := parseFile(t, "testdata/bbb-sample.html")

	if len(page.MissingDivisions) > 0 {
		t.Errorf("missing divisions: %v", page.MissingDivisions)
	}
	if got := len(page.Items); got != 41 {
		t.Errorf("got %d items, want 41", got)
	}
	counts := map[Division]int{}
	for i, it := range page.Items {
		if it.Position != counts[it.Division] {
			t.Errorf("item %d (%d): position %d, want %d", i, it.PanelID, it.Position, counts[it.Division])
		}
		counts[it.Division]++
	}
	want := map[Division]int{DivisionAll: 17, DivisionLower: 10, DivisionMiddle: 6, DivisionUpper: 8}
	for d, n := range want {
		if counts[d] != n {
			t.Errorf("%s: %d items, want %d", d, counts[d], n)
		}
	}

	var tuition *Item
	for i := range page.Items {
		if page.Items[i].PanelID == 67589 {
			tuition = &page.Items[i]
		}
	}
	if tuition == nil {
		t.Fatal("panel 67589 not found")
	}
	v := Normalize(*tuition, SignupDetector{Hosts: DefaultSignupHosts}, snapshotDate)
	if v.Title != "2027-28 Tuition Assistance Application" {
		t.Errorf("title = %q", v.Title)
	}
	if v.Subheader != "Deadline: Sunday, November 1" {
		t.Errorf("subheader = %q", v.Subheader)
	}
	if tuition.Division != DivisionAll || tuition.Position != 0 {
		t.Errorf("division/position = %s/%d", tuition.Division, tuition.Position)
	}
	if v.Deadline == nil || *v.Deadline != (Date{2026, time.November, 1}) {
		t.Errorf("deadline = %v", v.Deadline)
	}
	if !v.IsSignup {
		t.Error("tuition assistance should be a signup (clarityapp link)")
	}
}

// goldenItem is the golden-file form of one parsed and normalized item.
type goldenItem struct {
	PanelID     int64    `json:"panel_id"`
	Division    Division `json:"division"`
	Position    int      `json:"position"`
	Title       string   `json:"title"`
	Subheader   string   `json:"subheader"`
	BodyText    string   `json:"body_text"`
	ContentHash string   `json:"content_hash"`
	IsSignup    bool     `json:"is_signup"`
	SignupLinks []string `json:"signup_links"`
	Deadline    string   `json:"deadline,omitempty"`
}

// TestGolden pins the full parse and normalization of the sample page.
// Run with -update after an intended change and review the diff.
func TestGolden(t *testing.T) {
	page := parseFile(t, "testdata/bbb-sample.html")
	sd := SignupDetector{Hosts: DefaultSignupHosts}
	got := make([]goldenItem, 0, len(page.Items))
	for _, it := range page.Items {
		v := Normalize(it, sd, snapshotDate)
		g := goldenItem{
			PanelID: it.PanelID, Division: it.Division, Position: it.Position,
			Title: v.Title, Subheader: v.Subheader, BodyText: v.BodyText,
			ContentHash: v.ContentHash, IsSignup: v.IsSignup, SignupLinks: v.SignupLinks,
		}
		if v.Deadline != nil {
			g.Deadline = v.Deadline.String()
		}
		got = append(got, g)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(got); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()

	const golden = "testdata/bbb-sample.golden.json"
	if *update {
		if err := os.WriteFile(golden, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if string(want) != string(data) {
		t.Errorf("output differs from %s; run go test ./internal/bbb -update and review the diff", golden)
	}
}

// TestLocalFixtures parses every raw snapshot in docs/fixtures/. That
// directory is gitignored, so this only runs where snapshots were saved.
func TestLocalFixtures(t *testing.T) {
	files, _ := filepath.Glob("../../../docs/fixtures/*.html")
	if len(files) == 0 {
		t.Skip("no local snapshots in docs/fixtures/")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			page := parseFile(t, f)
			if len(page.MissingDivisions) > 0 {
				t.Errorf("missing divisions: %v", page.MissingDivisions)
			}
			if len(page.Items) == 0 {
				t.Error("no items")
			}
			for _, it := range page.Items {
				if it.Title == "" {
					t.Errorf("panel %d has no title", it.PanelID)
				}
			}
		})
	}
}
