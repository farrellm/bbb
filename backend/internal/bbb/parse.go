// Package bbb turns the Big Blue Bulletin page into items and compares them
// with what was seen before: fetch, parse, normalize, signup detection and
// the differ that produces new/edit/moved/removed events.
package bbb

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Division is one of the four sections of the page. The values are what the
// database stores and what preferences.divisions lists.
type Division string

// The four divisions.
const (
	DivisionAll    Division = "all"
	DivisionLower  Division = "lower"
	DivisionMiddle Division = "middle"
	DivisionUpper  Division = "upper"
)

// Divisions lists every division in page order.
var Divisions = []Division{DivisionAll, DivisionLower, DivisionMiddle, DivisionUpper}

// anchors maps each division to the id of the anchor that starts it.
var anchors = map[Division]string{
	DivisionAll:    "allschool",
	DivisionLower:  "lowerschool",
	DivisionMiddle: "middleschool",
	DivisionUpper:  "upperschool",
}

// Item is one panel as it appears on the page, before normalization.
type Item struct {
	PanelID   int64
	Division  Division
	Position  int // 0-based index within its division
	Title     string
	Subheader string // inner HTML of .fsElementHeaderContent
	BodyHTML  string // inner HTML of every body .fsElementContent, concatenated
}

// Page is the result of parsing one fetch.
type Page struct {
	Items []Item
	// MissingDivisions lists divisions whose anchor or panel group was not
	// found. Any entry makes the run suspect.
	MissingDivisions []Division
}

// Parse reads the BBB page and returns its items in document order.
func Parse(r io.Reader) (*Page, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	page := &Page{}
	seen := map[int64]bool{}
	for _, div := range Divisions {
		group := panelGroup(doc, anchors[div])
		if group == nil {
			page.MissingDivisions = append(page.MissingDivisions, div)
			continue
		}
		pos := 0
		var perr error
		group.Find("section.fsPanel").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			item, err := parsePanel(s)
			if err != nil {
				perr = err
				return false
			}
			// A panel can only live in one place; if Finalsite ever renders
			// one twice, keep the first so panel_id stays a key.
			if seen[item.PanelID] {
				return true
			}
			seen[item.PanelID] = true
			item.Division = div
			item.Position = pos
			pos++
			page.Items = append(page.Items, item)
			return true
		})
		if perr != nil {
			return nil, perr
		}
	}
	return page, nil
}

// panelGroup finds the accordion that follows the anchor with the given id:
// the anchor's enclosing div.fsContent, then the next div.fsPanelGroup
// sibling. Returns nil if either is missing.
func panelGroup(doc *goquery.Document, id string) *goquery.Selection {
	anchor := doc.Find("#" + id).First()
	if anchor.Length() == 0 {
		return nil
	}
	content := anchor.Closest("div.fsContent")
	if content.Length() == 0 {
		return nil
	}
	group := content.NextAllFiltered("div.fsPanelGroup").First()
	if group.Length() == 0 {
		return nil
	}
	return group
}

func parsePanel(s *goquery.Selection) (Item, error) {
	id, _ := s.Attr("id")
	n, ok := strings.CutPrefix(id, "fsEl_")
	if !ok {
		return Item{}, fmt.Errorf("unexpected panel id %q", id)
	}
	panelID, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		return Item{}, fmt.Errorf("panel id %q: %w", id, err)
	}

	title := strings.TrimSpace(s.Find("h2.fsElementTitle a").First().Text())
	sub, err := s.Find(".fsElementHeaderContent").First().Html()
	if err != nil {
		return Item{}, fmt.Errorf("panel %d subheader: %w", panelID, err)
	}

	var body strings.Builder
	var berr error
	s.Find(".fsElementContent .fsElement.fsContent .fsElementContent").Each(func(_ int, c *goquery.Selection) {
		h, err := c.Html()
		if err != nil {
			berr = err
			return
		}
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(strings.TrimSpace(h))
	})
	if berr != nil {
		return Item{}, fmt.Errorf("panel %d body: %w", panelID, berr)
	}

	return Item{
		PanelID:   panelID,
		Title:     title,
		Subheader: strings.TrimSpace(sub),
		BodyHTML:  body.String(),
	}, nil
}
