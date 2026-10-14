package bbb

import (
	"slices"
	"testing"
	"time"
)

func TestDetectSignup(t *testing.T) {
	sd := SignupDetector{Hosts: DefaultSignupHosts}
	tests := []struct {
		title, sub, body string
		want             bool
		links            []string
	}{
		{title: "Friday Night Lights: Join Us and Sign Up to Help!", want: true},
		{title: "Field Hockey Lights Up Babbitt Field", sub: "Thursday, October 15", want: false},
		{title: "Grade 2 Parent Social—RSVP Today!", want: true},
		{title: "Lunchroom Volunteer Program", want: true},
		{title: "Tickets on Sale Now", want: true},
		{title: "Applications Open for the Global Program", want: true},
		{title: "Signup sheet", want: true},
		{title: "Blue and White Days", sub: "Register by October 31", want: true},
		{title: "Blue and White Days", sub: "Wednesday, October 21", want: false},
		{
			title: "Faculty Breakfast", want: true,
			body: `<a href="https://www.signupgenius.com/go/abc#/">here</a> and <a href="https://www.signupgenius.com/go/abc#/">again</a>
				<a href="https://www.pingry.org/news">news</a> <a href="mailto:a@b.org">mail</a>`,
			links: []string{"https://www.signupgenius.com/go/abc#/"},
		},
		{
			title: "Coffee", want: true,
			body:  `<a href="https://docs.google.com/forms/d/e/1/viewform">form</a> <a href="https://docs.google.com/document/d/2">doc</a> <a href="https://forms.gle/xyz">short</a>`,
			links: []string{"https://docs.google.com/forms/d/e/1/viewform", "https://forms.gle/xyz"},
		},
		{
			title: "Play", want: true,
			body:  `<a href="https://cur8.com/1/project/2">tickets</a> <a href="https://www.eventbrite.com/e/1">eb</a> <a href="https://pingry.campbrainregistration.com/Registration/1">camp</a>`,
			links: []string{"https://cur8.com/1/project/2", "https://www.eventbrite.com/e/1", "https://pingry.campbrainregistration.com/Registration/1"},
		},
		{
			title: "Dress Days", want: false,
			body: `<a href="https://portals.veracross.com/pingry/email/1">email</a> <a href="https://notcur8.com/x">x</a> <a href="https://evil.com/docs.google.com/forms">x</a>`,
		},
		{
			title: "Open House", want: true,
			body:  `<a href="https://events.veracross.com/pingry/543">open house</a>`,
			links: []string{"https://events.veracross.com/pingry/543"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got, links := sd.Detect(tt.title, tt.sub, tt.body)
			if got != tt.want {
				t.Errorf("Detect = %v, want %v", got, tt.want)
			}
			if !slices.Equal(links, tt.links) {
				t.Errorf("links = %q, want %q", links, tt.links)
			}
		})
	}
}

func TestExtractDeadline(t *testing.T) {
	oct := Date{2026, time.October, 14}
	tests := []struct {
		sub  string
		now  Date
		want string
	}{
		{"Deadline: Sunday, November 1", oct, "2026-11-01"},
		{"Deadline: December 31", oct, "2026-12-31"},
		{"Attn: Form VI Students\nSubmit All Materials by October 19", oct, "2026-10-19"},
		{"Deadline for AP Exam Only Sign-Up Form: Monday, November 9, 3:00 p.m.\nDeadline for Requests: Friday, October 30", oct, "2026-11-09"},
		{"Wednesday, November 18 (Upper School)\nTuesday, January 12 (Middle School)", oct, "2026-11-18"},
		{"Tuesday, January 12", oct, "2027-01-12"},
		{"Deadline: Friday, October 30", Date{2027, time.March, 1}, "2026-10-30"},
		{"Due Sept. 4", Date{2026, time.August, 20}, "2026-09-04"},
		{"Saturday, November 14, 2026\nRegister by October 31", oct, "2026-11-14"},
		{"Deadline: 11/9", oct, "2026-11-09"},
		{"Deadline: 1/5/27", oct, "2027-01-05"},
		{"Starts October 15. Deadline: October 20", oct, "2026-10-20"},
		{"Attn: Form I and II Families", oct, ""},
		{"February 30", oct, ""},
		{"", oct, ""},
	}
	for _, tt := range tests {
		t.Run(tt.sub, func(t *testing.T) {
			got := ""
			if d := ExtractDeadline(tt.sub, tt.now); d != nil {
				got = d.String()
			}
			if got != tt.want {
				t.Errorf("ExtractDeadline(%q) = %q, want %q", tt.sub, got, tt.want)
			}
		})
	}
}
