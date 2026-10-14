package bbb

import (
	"slices"
	"strings"
	"testing"
)

func seg(t SegmentType, s string) Segment { return Segment{t, s} }

// sides rebuilds before (dropping adds) and after (dropping removes).
func sides(segs []Segment) (string, string) {
	var a, b strings.Builder
	for _, s := range segs {
		if s.Type != SegAdd {
			a.WriteString(s.Text)
		}
		if s.Type != SegRemove {
			b.WriteString(s.Text)
		}
	}
	return a.String(), b.String()
}

func TestTokenize(t *testing.T) {
	tests := map[string][]string{
		"Hi, world!":   {"Hi", ",", " ", "world", "!"},
		"don't":        {"don't"},
		"it’s re-read": {"it’s", " ", "re-read"},
		"'tis":         {"'", "tis"},
		"well--known":  {"well", "-", "-", "known"},
		"résumé":       {"résumé"},
		"a b":          {"a", " ", "b"},
		"":             nil,
	}
	for in, want := range tests {
		if got := diffToken.FindAllString(in, -1); !slices.Equal(got, want) {
			t.Errorf("tokenize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWordDiff(t *testing.T) {
	tests := []struct {
		name, a, b string
		want       []Segment
	}{
		{"identical", "same text", "same text", []Segment{seg(SegEqual, "same text")}},
		{"both empty", "", "", nil},
		{"empty before", "", "new words", []Segment{seg(SegAdd, "new words")}},
		{"empty after", "old words", "", []Segment{seg(SegRemove, "old words")}},
		{
			"adjacent words group", "the quick brown fox", "the slow red fox",
			[]Segment{seg(SegEqual, "the "), seg(SegRemove, "quick brown"), seg(SegAdd, "slow red"), seg(SegEqual, " fox")},
		},
		{
			"punctuation stays put", "room.", "room and smiled.",
			[]Segment{seg(SegEqual, "room"), seg(SegAdd, " and smiled"), seg(SegEqual, ".")},
		},
		{
			"newline equality is not absorbed", "a\nb", "c\nd",
			[]Segment{seg(SegRemove, "a"), seg(SegAdd, "c"), seg(SegEqual, "\n"), seg(SegRemove, "b"), seg(SegAdd, "d")},
		},
		{
			"preserved phrase stays", "one two three four five six", "uno two three four five seis",
			[]Segment{seg(SegRemove, "one"), seg(SegAdd, "uno"), seg(SegEqual, " two three four five "), seg(SegRemove, "six"), seg(SegAdd, "seis")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordDiff(tt.a, tt.b)
			if !slices.Equal(got, tt.want) {
				t.Errorf("WordDiff(%q, %q)\n got %+v\nwant %+v", tt.a, tt.b, got, tt.want)
			}
			if a, b := sides(got); a != tt.a || b != tt.b {
				t.Errorf("sides = %q, %q", a, b)
			}
		})
	}
}

func TestWordDiffReconstructs(t *testing.T) {
	pairs := [][2]string{
		{"Volunteers needed for lunch on Mondays.", "Volunteers are needed for lunch on Mondays and Fridays!"},
		{"Deadline: Friday, October 30", "Deadline: Monday, November 2"},
		{"A\n\nB C D\n\nE", "A\n\nB X D\n\nE F"},
		{"completely different text here", "nothing in common at all"},
	}
	for _, p := range pairs {
		if a, b := sides(WordDiff(p[0], p[1])); a != p[0] || b != p[1] {
			t.Errorf("WordDiff(%q, %q) rebuilds %q, %q", p[0], p[1], a, b)
		}
	}
}

func TestDiffLinesRefined(t *testing.T) {
	a := "Intro line\nSecond line old words\nThird\n"
	b := "Intro line\nSecond line new words\nThird\nFourth\n"
	sink := &segmentSink{}
	diffLinesRefined(lineToken.FindAllString(a, -1), lineToken.FindAllString(b, -1), sink.push)
	segs := groupChanges(sink.segs)
	if ga, gb := sides(segs); ga != a || gb != b {
		t.Fatalf("rebuilds %q, %q", ga, gb)
	}
	want := []Segment{
		seg(SegEqual, "Intro line\nSecond line "), seg(SegRemove, "old"), seg(SegAdd, "new"),
		seg(SegEqual, " words\nThird\n"), seg(SegAdd, "Fourth\n"),
	}
	if !slices.Equal(segs, want) {
		t.Errorf("got %+v\nwant %+v", segs, want)
	}
}

func TestWordCount(t *testing.T) {
	tests := map[string]int{
		"":                   0,
		"don't stop":         2,
		" , . ! ":            0,
		"re-read the résumé": 3,
		"8:45–9:45 a.m.":     6,
	}
	for in, want := range tests {
		if got := WordCount(in); got != want {
			t.Errorf("WordCount(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestWordDelta(t *testing.T) {
	r, a := WordDelta("Join us Friday.", "Join us Friday!")
	if r != 0 || a != 0 {
		t.Errorf("punctuation-only change: -%d +%d", r, a)
	}
	r, a = WordDelta("the quick brown fox", "the slow red fox")
	if r != 2 || a != 2 {
		t.Errorf("got -%d +%d, want -2 +2", r, a)
	}
	// " fox" sits between two edits and is small enough to be absorbed, so
	// it counts on both sides: counts describe the rendered diff, as in
	// wordDiff.md.
	r, a = WordDelta("the quick brown fox", "the slow red fox jumps")
	if r != 3 || a != 4 {
		t.Errorf("got -%d +%d, want -3 +4", r, a)
	}
}
