// Command sanitize-fixture turns a raw snapshot of the BBB page into the
// committed test fixture. It keeps the Finalsite markup, panel ids and link
// hosts, but replaces all text with placeholder words, every mailto: with
// example@example.org, and link paths past their first segment with "x".
// Scripts, styles, comments and free-text attributes are emptied.
//
// One panel (fsEl_67589, a generic tuition-assistance notice) keeps its text
// so tests can check a real title and subheader.
//
// Usage: sanitize-fixture < docs/fixtures/raw.html > internal/bbb/testdata/bbb-sample.html
package main

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const keepPanel = "fsEl_67589"

var lorem = strings.Fields(`lorem ipsum dolor sit amet consectetur adipiscing elit sed do
	eiusmod tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam quis
	nostrud exercitation ullamco laboris nisi aliquip ex ea commodo consequat`)

var (
	letters = regexp.MustCompile(`\p{L}+`)
	digits  = regexp.MustCompile(`\p{N}`)
)

// Attributes whose values are free text or embedded content.
var clearAttrs = map[string]bool{
	"alt": true, "title": true, "aria-label": true, "placeholder": true,
	"content": true, "srcset": true, "value": true,
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sanitize-fixture:", err)
		os.Exit(1)
	}
}

func run() error {
	doc, err := html.Parse(os.Stdin)
	if err != nil {
		return err
	}
	s := &sanitizer{}
	s.walk(doc, false)
	return html.Render(os.Stdout, doc)
}

type sanitizer struct{ word int }

func (s *sanitizer) walk(n *html.Node, keep bool) {
	if n.Type == html.ElementNode {
		if id := attr(n, "id"); id == keepPanel {
			keep = true
		}
		s.attrs(n)
		if n.DataAtom == atom.Script || n.DataAtom == atom.Style || n.DataAtom == atom.Noscript {
			for c := n.FirstChild; c != nil; {
				next := c.NextSibling
				n.RemoveChild(c)
				c = next
			}
			return
		}
	}

	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch c.Type {
		case html.CommentNode:
			n.RemoveChild(c)
		case html.TextNode:
			if !keep {
				c.Data = s.text(c.Data)
			}
		default:
			s.walk(c, keep)
		}
		c = next
	}
}

// text swaps each run of letters for a placeholder word and each digit for
// 0, keeping whitespace (including &nbsp;) and punctuation.
func (s *sanitizer) text(t string) string {
	t = letters.ReplaceAllStringFunc(t, func(string) string {
		w := lorem[s.word%len(lorem)]
		s.word++
		return w
	})
	return digits.ReplaceAllString(t, "0")
}

func (s *sanitizer) attrs(n *html.Node) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		switch {
		case strings.HasPrefix(a.Key, "data-") && a.Key != "data-use-new":
			continue
		case clearAttrs[a.Key]:
			a.Val = ""
		case a.Key == "href" || a.Key == "action":
			a.Val = sanitizeURL(a.Val)
		}
		out = append(out, a)
	}
	n.Attr = out
}

func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "#") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "#"
	}
	switch u.Scheme {
	case "mailto":
		return "mailto:example@example.org"
	case "tel", "sms":
		return u.Scheme + ":0000000000"
	case "http", "https", "":
	default:
		return "#"
	}
	segs := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	path := "/"
	if segs[0] != "" {
		path += segs[0]
		if len(segs) > 1 {
			path += "/x"
		}
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: path, Fragment: u.Fragment}).String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
