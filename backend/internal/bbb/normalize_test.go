package bbb

import "testing"

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "<p>Hello world</p>", "Hello world"},
		{"nbsp", "<p>Hello&nbsp;&nbsp; world&nbsp;</p>", "Hello world"},
		{"source newlines are spaces", "<p>Hello\n\tworld</p>", "Hello world"},
		{"br", "<p>Line one<br />\nLine two</p>", "Line one\nLine two"},
		{"nbsp-only line dropped", "<p>One<br />\n&nbsp;<br />\nTwo</p>", "One\n\nTwo"},
		{"empty paragraph dropped", "<p>One</p><p>&nbsp;</p><p></p><p>Two</p>", "One\n\nTwo"},
		{"link keeps target", `<p>Please <a href="https://x.org/f">RSVP here</a>.</p>`, "Please RSVP here <https://x.org/f>."},
		{"link text equal to url", `<a href="https://x.org">https://x.org</a>`, "https://x.org"},
		{"in-page link has no target", `<a href="#top">Top</a>`, "Top"},
		{"anchor without href", `<a id="x">Name</a>`, "Name"},
		{"list", "<ul>\n\t<li>One</li>\n\t<li>Two&nbsp;</li>\n</ul>", "• One\n• Two"},
		{"paragraph then list", "<p>Dates:</p>\n<ul><li>A</li><li>B</li></ul><p>After</p>", "Dates:\n\n• A\n• B\n\nAfter"},
		{"entities", "<p>Pingry&rsquo;s 8:45&ndash;9:45 &amp; more</p>", "Pingry’s 8:45–9:45 & more"},
		{"script dropped", "<p>Hi</p><script>alert(1)</script>", "Hi"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTMLToText(tt.in); got != tt.want {
				t.Errorf("HTMLToText(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeHashIgnoresWhitespaceNoise(t *testing.T) {
	sd := SignupDetector{Hosts: DefaultSignupHosts}
	a := Item{Title: "Book Fair", Subheader: "<p>Friday</p>", BodyHTML: "<p>Come to the fair.</p>"}
	b := Item{Title: "Book\u00a0Fair ", Subheader: "<p>Friday&nbsp;</p>\n", BodyHTML: "<p>Come  to&nbsp;the fair.</p><p>&nbsp;</p>"}
	b.Title = "Book Fair "
	va, vb := Normalize(a, sd, snapshotDate), Normalize(b, sd, snapshotDate)
	if va.ContentHash != vb.ContentHash {
		t.Errorf("hashes differ:\n%+v\n%+v", va, vb)
	}

	c := a
	c.BodyHTML = `<p>Come to the <a href="https://x.org">fair</a>.</p>`
	if Normalize(c, sd, snapshotDate).ContentHash == va.ContentHash {
		t.Error("adding a link should change the hash")
	}
}
