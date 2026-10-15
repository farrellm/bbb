package bbb

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Version is the normalized form of an Item: the text that gets diffed and
// hashed, plus what signup detection found in it.
type Version struct {
	Title       string
	Subheader   string // plain text
	BodyHTML    string // as scraped; sanitized only on the way to the client
	BodyText    string
	ContentHash string
	IsSignup    bool
	SignupLinks []string
	Deadline    *Date
}

// Normalize converts a parsed Item into a Version. now anchors deadline
// year inference to the current school year.
func Normalize(it Item, sd SignupDetector, now Date) Version {
	title := collapseLine(it.Title)
	sub := HTMLToText(it.Subheader)
	body := HTMLToText(it.BodyHTML)
	v := Version{
		Title:       title,
		Subheader:   sub,
		BodyHTML:    it.BodyHTML,
		BodyText:    body,
		ContentHash: ContentHash(title, sub, body),
	}
	v.IsSignup, v.SignupLinks = sd.Detect(title, sub, it.BodyHTML)
	v.Deadline = ExtractDeadline(sub, now)
	return v
}

// ContentHash is sha256(title \n subheader \n body_text), hex encoded.
func ContentHash(title, subheader, body string) string {
	sum := sha256.Sum256([]byte(title + "\n" + subheader + "\n" + body))
	return hex.EncodeToString(sum[:])
}

var (
	spaceRun   = regexp.MustCompile(`[ \t]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)

	sourceSpace = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")
)

// HTMLToText renders an HTML fragment as readable text. Links keep their
// target as "text <url>" so a changed link counts as an edit, list items
// start with "• ", &nbsp; becomes a plain space, runs of spaces collapse,
// lines are trimmed and empty paragraphs disappear.
func HTMLToText(fragment string) string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		// The tokenizer accepts any input; ParseFragment only fails on a
		// reader error, which a strings.Reader never returns.
		return collapseLine(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		writeText(&b, n)
	}
	return tidy(b.String())
}

func writeText(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		// Indentation between list items or table rows would otherwise
		// leave blank lines between them.
		if n.Parent != nil && isContainer(n.Parent.DataAtom) && strings.TrimSpace(n.Data) == "" {
			return
		}
		// Line breaks in the HTML source are just whitespace; only <br>
		// and block elements start a new line.
		b.WriteString(sourceSpace.Replace(n.Data))
	case html.ElementNode:
		writeElement(b, n)
	default:
		writeChildren(b, n)
	}
}

func writeElement(b *strings.Builder, n *html.Node) {
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Template:
	case atom.Br:
		b.WriteString("\n")
	case atom.Li:
		// No trailing newline: the next item or the end of the list
		// starts the next line, and a doubled one would read as a blank
		// line between items.
		b.WriteString("\n• ")
		writeChildren(b, n)
	case atom.A:
		start := b.Len()
		writeChildren(b, n)
		text := strings.TrimSpace(b.String()[start:])
		if href := linkTarget(n); href != "" && href != text {
			b.WriteString(" <" + href + ">")
		}
	default:
		if !isBlock(n.DataAtom) {
			writeChildren(b, n)
			return
		}
		b.WriteString("\n")
		writeChildren(b, n)
		b.WriteString("\n")
		if n.DataAtom == atom.P {
			b.WriteString("\n")
		}
	}
}

func writeChildren(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeText(b, c)
	}
}

// linkTarget returns the href worth keeping in the text, or "" for anchors
// with no destination or that only jump within the page.
func linkTarget(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "href" {
			href := strings.TrimSpace(a.Val)
			if href == "" || strings.HasPrefix(href, "#") {
				return ""
			}
			return href
		}
	}
	return ""
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Ul, atom.Ol, atom.Table, atom.Tr, atom.Blockquote,
		atom.Section, atom.Header, atom.Footer, atom.Article, atom.Figure, atom.Hr,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre, atom.Dl, atom.Dt, atom.Dd:
		return true
	default:
		return false
	}
}

// isContainer reports elements whose direct text children are only ever
// source indentation.
func isContainer(a atom.Atom) bool {
	switch a {
	case atom.Ul, atom.Ol, atom.Table, atom.Thead, atom.Tbody, atom.Tfoot, atom.Tr, atom.Dl:
		return true
	default:
		return false
	}
}

// tidy applies the whitespace rules to rendered text: &nbsp; is a space,
// runs of spaces collapse, every line is trimmed and blank lines collapse to
// one.
func tidy(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRun.ReplaceAllString(l, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// collapseLine reduces any whitespace run, &nbsp; included, to one space.
func collapseLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
}
