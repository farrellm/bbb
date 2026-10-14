package bbb

import (
	"regexp"
	"strings"
)

// This file is a Go port of nisaba's frontend/src/lib/wordDiff.ts, which the
// frontend copies as-is. The server uses it to count changed words for the
// edit noise filter and notification text, so the counts must match what the
// diff view shows. The algorithm is documented step by step in the
// frontend's wordDiff.md; keep all three in sync.

// SegmentType marks a diff segment as unchanged, removed or added.
type SegmentType int

// The segment types.
const (
	SegEqual SegmentType = iota
	SegRemove
	SegAdd
)

// Segment is one run of text in a word diff.
type Segment struct {
	Type SegmentType
	Text string
}

// jsSpace is JavaScript's \s, which unlike RE2's also covers Unicode spaces
// such as U+00A0.
const jsSpace = `\s\v\p{Z}\x{FEFF}`

var (
	diffToken = regexp.MustCompile(`[` + jsSpace + `]+|[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*|[^` + jsSpace + `\p{L}\p{N}]`)
	lineToken = regexp.MustCompile(`\n+|[^\n]+`)
	wordRun   = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*`)
	allSpace  = regexp.MustCompile(`^[` + jsSpace + `]+$`)
)

// maxLCSCells caps the LCS table size before dropping to line granularity.
const maxLCSCells = 25_000_000

// WordCount counts word runs in s, ignoring whitespace and punctuation.
func WordCount(s string) int {
	return len(wordRun.FindAllStringIndex(s, -1))
}

// WordDelta returns the words removed and added between before and after,
// counted the way the diff view's "−N / +M words" summary counts them.
func WordDelta(before, after string) (removed, added int) {
	for _, s := range WordDiff(before, after) {
		switch s.Type {
		case SegRemove:
			removed += WordCount(s.Text)
		case SegAdd:
			added += WordCount(s.Text)
		case SegEqual:
		}
	}
	return removed, added
}

type segmentSink struct{ segs []Segment }

func (s *segmentSink) push(t SegmentType, text string) {
	if text == "" {
		return
	}
	if n := len(s.segs); n > 0 && s.segs[n-1].Type == t {
		s.segs[n-1].Text += text
		return
	}
	s.segs = append(s.segs, Segment{t, text})
}

// WordDiff computes a word-level diff between two strings.
func WordDiff(before, after string) []Segment {
	a := diffToken.FindAllString(before, -1)
	b := diffToken.FindAllString(after, -1)
	start, midA, midB := trimCommonEnds(a, b)

	out := &segmentSink{}
	out.push(SegEqual, strings.Join(a[:start], ""))

	if fitsCap(midA, midB) {
		diffTokens(midA, midB, out.push)
	} else {
		lineA := lineToken.FindAllString(strings.Join(midA, ""), -1)
		lineB := lineToken.FindAllString(strings.Join(midB, ""), -1)
		if fitsCap(lineA, lineB) {
			diffLinesRefined(lineA, lineB, out.push)
		} else {
			out.push(SegRemove, strings.Join(midA, ""))
			out.push(SegAdd, strings.Join(midB, ""))
		}
	}

	out.push(SegEqual, strings.Join(a[start+len(midA):], ""))
	return groupChanges(out.segs)
}

func trimCommonEnds(a, b []string) (int, []string, []string) {
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}
	return start, a[start:endA], b[start:endB]
}

func fitsCap(a, b []string) bool {
	return (len(a)+1)*(len(b)+1) <= maxLCSCells
}

func diffLinesRefined(lineA, lineB []string, push func(SegmentType, string)) {
	var removed, added strings.Builder
	flush := func() {
		if removed.Len() == 0 && added.Len() == 0 {
			return
		}
		a := diffToken.FindAllString(removed.String(), -1)
		b := diffToken.FindAllString(added.String(), -1)
		start, midA, midB := trimCommonEnds(a, b)
		push(SegEqual, strings.Join(a[:start], ""))
		if fitsCap(midA, midB) {
			diffTokens(midA, midB, push)
		} else {
			push(SegRemove, strings.Join(midA, ""))
			push(SegAdd, strings.Join(midB, ""))
		}
		push(SegEqual, strings.Join(a[start+len(midA):], ""))
		removed.Reset()
		added.Reset()
	}
	diffTokens(lineA, lineB, func(t SegmentType, text string) {
		switch t {
		case SegEqual:
			flush()
			push(SegEqual, text)
		case SegRemove:
			removed.WriteString(text)
		case SegAdd:
			added.WriteString(text)
		}
	})
	flush()
}

func diffTokens(a, b []string, push func(SegmentType, string)) {
	n, m := len(a), len(b)
	width := m + 1
	lcs := make([]int32, (n+1)*width)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
			} else {
				lcs[i*width+j] = max(lcs[(i+1)*width+j], lcs[i*width+j+1])
			}
		}
	}

	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			push(SegEqual, a[i])
			i++
			j++
		case lcs[(i+1)*width+j] >= lcs[i*width+j+1]:
			push(SegRemove, a[i])
			i++
		default:
			push(SegAdd, b[j])
			j++
		}
	}
	for ; i < n; i++ {
		push(SegRemove, a[i])
	}
	for ; j < m; j++ {
		push(SegAdd, b[j])
	}
}

// chunk is groupChanges' working form: equal text, or an edit that renders
// as one remove block followed by one add block.
type chunk struct {
	edit   bool
	text   string // equal text
	remove string
	add    string
}

func groupChanges(segs []Segment) []Segment {
	var chunks []*chunk
	for _, s := range segs {
		var last *chunk
		if len(chunks) > 0 {
			last = chunks[len(chunks)-1]
		}
		switch {
		case s.Type == SegEqual:
			chunks = append(chunks, &chunk{text: s.Text})
		case last != nil && last.edit:
			if s.Type == SegRemove {
				last.remove += s.Text
			} else {
				last.add += s.Text
			}
		case s.Type == SegRemove:
			chunks = append(chunks, &chunk{edit: true, remove: s.Text})
		default:
			chunks = append(chunks, &chunk{edit: true, add: s.Text})
		}
	}

	for merged := true; merged; {
		merged = false
		for i := 1; i < len(chunks)-1; i++ {
			eq, prev, next := chunks[i], chunks[i-1], chunks[i+1]
			if eq.edit || !prev.edit || !next.edit || !absorbable(eq.text, prev, next) {
				continue
			}
			prev.remove += eq.text + next.remove
			prev.add += eq.text + next.add
			chunks = append(chunks[:i], chunks[i+2:]...)
			merged = true
		}
	}

	var out []Segment
	for _, c := range chunks {
		if !c.edit {
			out = append(out, Segment{SegEqual, c.text})
			continue
		}
		if c.remove != "" {
			out = append(out, Segment{SegRemove, c.remove})
		}
		if c.add != "" {
			out = append(out, Segment{SegAdd, c.add})
		}
	}
	return out
}

func absorbable(eq string, prev, next *chunk) bool {
	if strings.Contains(eq, "\n") {
		return false
	}
	if allSpace.MatchString(eq) {
		return true
	}
	wc := WordCount(eq)
	return wc <= max(WordCount(prev.remove), WordCount(prev.add)) &&
		wc <= max(WordCount(next.remove), WordCount(next.add))
}
