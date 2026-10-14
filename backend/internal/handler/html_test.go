package handler

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	c := newBodyCleaner(pageURL)
	got := c.Clean(`<p style="color:red" onclick="x()">Hi <a href="/fs/doc.pdf">doc</a>` +
		` <a href="#top">top</a> <a href="javascript:alert(1)">bad</a><script>alert(1)</script>` +
		` <img src="/img.png"> <a href="mailto:a@example.org">mail</a></p>`)
	for _, want := range []string{
		`href="https://www.pingry.org/fs/doc.pdf"`,
		`target="_blank"`,
		`noopener`,
		`noreferrer`,
		`src="https://www.pingry.org/img.png"`,
		`href="#top"`,
		`href="mailto:a@example.org"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	for _, bad := range []string{"script", "onclick", "javascript", "style="} {
		if strings.Contains(got, bad) {
			t.Errorf("kept %s in %s", bad, got)
		}
	}
}

func TestSignupLinks(t *testing.T) {
	c := newBodyCleaner(pageURL)
	body := `<a href="https://www.signupgenius.com/go/1">Sign up here</a>
		<a href="https://forms.gle/x"> RSVP
		now</a> <a href="https://www.eventbrite.com/e/2">Get your tickets</a>
		<a href="https://campbrain.com/r">Registration form</a> <a href="https://jotform.com/z">form</a>`
	got := c.signupLinks(body, []string{
		"https://www.signupgenius.com/go/1", "https://forms.gle/x", "https://www.eventbrite.com/e/2",
		"https://campbrain.com/r", "https://jotform.com/z", "javascript:alert(1)",
	})
	want := []signupLinkJSON{
		{"https://www.signupgenius.com/go/1", "Sign up", "Sign up here"},
		{"https://forms.gle/x", "RSVP", "RSVP now"},
		{"https://www.eventbrite.com/e/2", "Buy tickets", "Get your tickets"},
		{"https://campbrain.com/r", "Register", "Registration form"},
		{"https://jotform.com/z", "Sign up", "form"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                   "/",
		"/items/5?diff=1..2": "/items/5?diff=1..2",
		"//evil.example":     "/",
		"/\\evil.example":    "/",
		"https://evil":       "/",
		"items":              "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToken(t *testing.T) {
	if token("abc-_09XY") != "abc-_09XY" || token("a b") != "" || token("a/b") != "" ||
		token(strings.Repeat("a", 65)) != "" {
		t.Error("token filter")
	}
}
