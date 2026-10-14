package handler

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Body HTML is stored as scraped and cleaned only on its way to the client,
// so a stricter policy later applies to every stored version.
type bodyCleaner struct {
	base   *url.URL
	policy *bluemonday.Policy
}

// newBodyCleaner resolves relative links against pageURL (the BBB page), so
// they keep pointing at pingry.org rather than at this app.
func newBodyCleaner(pageURL string) *bodyCleaner {
	base, err := url.Parse(pageURL)
	if err != nil {
		base = &url.URL{}
	}
	p := bluemonday.UGCPolicy()
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	// Links leave the app: in a new tab, without an opener or referrer.
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)
	return &bodyCleaner{base: base, policy: p}
}

// Clean returns fragment safe to insert with innerHTML.
func (c *bodyCleaner) Clean(fragment string) string {
	return c.policy.Sanitize(c.absolutize(fragment))
}

// absolutize rewrites relative href and src attributes against the page
// URL. Fragments that fail to parse are returned as they are; the sanitizer
// still runs on them.
func (c *bodyCleaner) absolutize(fragment string) string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		return fragment
	}
	var b strings.Builder
	for _, n := range nodes {
		c.rewrite(n)
		if err := html.Render(&b, n); err != nil {
			return fragment
		}
	}
	return b.String()
}

func (c *bodyCleaner) rewrite(n *html.Node) {
	if n.Type == html.ElementNode {
		for i, a := range n.Attr {
			if a.Key != "href" && a.Key != "src" {
				continue
			}
			v := strings.TrimSpace(a.Val)
			if v == "" || strings.HasPrefix(v, "#") {
				continue
			}
			if u, err := c.base.Parse(v); err == nil {
				n.Attr[i].Val = u.String()
			}
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.rewrite(ch)
	}
}

// Labels for signup tear-off tabs, from the link text. The first match
// wins; anything else is a plain "Sign up".
var linkLabels = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`(?i)\brsvp`), "RSVP"},
	{regexp.MustCompile(`(?i)\btickets?\b`), "Buy tickets"},
	{regexp.MustCompile(`(?i)\bregist`), "Register"},
	{regexp.MustCompile(`(?i)\bappl(y|ication)`), "Apply"},
	{regexp.MustCompile(`(?i)\bvolunteer`), "Volunteer"},
}

// signupLinks pairs each signup URL with the text of the first link to it
// in the body and a short label for its tab. URLs are as stored (they were
// already absolute when detected), and only http(s) ones are kept.
func (c *bodyCleaner) signupLinks(bodyHTML string, urls []string) []signupLinkJSON {
	text := linkTexts(bodyHTML)
	out := make([]signupLinkJSON, 0, len(urls))
	for _, u := range urls {
		if pu, err := url.Parse(u); err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
			continue
		}
		t := text[u]
		label := "Sign up"
		for _, l := range linkLabels {
			if l.re.MatchString(t) {
				label = l.label
				break
			}
		}
		out = append(out, signupLinkJSON{URL: u, Label: label, Text: t})
	}
	return out
}

// linkTexts maps each href in fragment to the collapsed text of its first
// anchor.
func linkTexts(fragment string) map[string]string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	out := map[string]string{}
	if err != nil {
		return out
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			for _, a := range n.Attr {
				href := strings.TrimSpace(a.Val)
				if a.Key == "href" && out[href] == "" {
					out[href] = strings.Join(strings.Fields(textOf(n)), " ")
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return out
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(textOf(ch))
	}
	return b.String()
}
