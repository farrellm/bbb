package bbb

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DefaultSignupHosts are the link destinations that mark an item as a signup.
// A pattern is one of:
//   - "host/path": the host (or a subdomain) and a path prefix, e.g. docs.google.com/forms
//   - "*.domain" or "domain.tld": that domain or any subdomain
//   - "label": a bare name matched against any label of the host, e.g. eventbrite
var DefaultSignupHosts = []string{
	"signupgenius.com",
	"docs.google.com/forms",
	"forms.gle",
	"punchbowl.com",
	"events.veracross.com", // portals.veracross.com hosts email archives, not forms
	"campbrain.com",
	"campbrainregistration.com",
	"helperhelper.com",
	"cur8",
	"clarityapp.com",
	"eventbrite",
	"jotform",
}

var signupKeywords = regexp.MustCompile(`(?i)\b(sign[\s-]?up|register|registration|rsvp|volunteer|tickets?|apply|application)`)

// SignupDetector decides whether an item asks parents to sign up for
// something, and which of its links are the signup forms.
type SignupDetector struct {
	Hosts []string
}

// Detect reports whether the item is a signup and returns its signup links
// in order of appearance, without duplicates. Either a link to a known
// signup host or a keyword in the title or subheader is enough.
func (d SignupDetector) Detect(title, subheader, bodyHTML string) (bool, []string) {
	var links []string
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(bodyHTML))
	if err == nil {
		doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			href := strings.TrimSpace(a.AttrOr("href", ""))
			if d.isSignupURL(href) && !slices.Contains(links, href) {
				links = append(links, href)
			}
		})
	}
	return len(links) > 0 || signupKeywords.MatchString(title+"\n"+subheader), links
}

func (d SignupDetector) isSignupURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range d.Hosts {
		if matchHost(strings.ToLower(p), host, u.EscapedPath()) {
			return true
		}
	}
	return false
}

func matchHost(pattern, host, path string) bool {
	if h, prefix, ok := strings.Cut(pattern, "/"); ok {
		return inDomain(host, h) && strings.HasPrefix(path, "/"+prefix)
	}
	if d, ok := strings.CutPrefix(pattern, "*."); ok {
		return inDomain(host, d)
	}
	if strings.Contains(pattern, ".") {
		return inDomain(host, pattern)
	}
	return slices.Contains(strings.Split(host, "."), pattern)
}

func inDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// Date is a calendar date with no time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// Time returns midnight UTC on the date.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

var (
	deadlineLabel = regexp.MustCompile(`(?i)deadline[^:\n]*:\s*(.+)`)
	monthDay      = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+(\d{1,2})(?:st|nd|rd|th)?\b(?:,?\s+(\d{4})\b)?`)
	numericDate   = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{2}|\d{4}))?\b`)
)

var monthByPrefix = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

// ExtractDeadline finds the date after "Deadline:" in the subheader, or else
// the first date in it. A date without a year gets the year that puts it in
// the school year containing now (July through June). Returns nil when no
// date is found.
func ExtractDeadline(subheader string, now Date) *Date {
	if m := deadlineLabel.FindStringSubmatch(subheader); m != nil {
		if d := firstDate(m[1], now); d != nil {
			return d
		}
	}
	return firstDate(subheader, now)
}

func firstDate(s string, now Date) *Date {
	type match struct {
		at   int
		date Date
	}
	var best *match
	consider := func(at int, d Date, ok bool) {
		if ok && (best == nil || at < best.at) {
			best = &match{at, d}
		}
	}

	if m := monthDay.FindStringSubmatchIndex(s); m != nil {
		month := monthByPrefix[strings.ToLower(s[m[2]:m[3]])]
		day, _ := strconv.Atoi(s[m[4]:m[5]])
		year := 0
		if m[6] >= 0 {
			year, _ = strconv.Atoi(s[m[6]:m[7]])
		}
		d, ok := makeDate(year, month, day, now)
		consider(m[0], d, ok)
	}
	if m := numericDate.FindStringSubmatchIndex(s); m != nil {
		mo, _ := strconv.Atoi(s[m[2]:m[3]])
		day, _ := strconv.Atoi(s[m[4]:m[5]])
		year := 0
		if m[6] >= 0 {
			year, _ = strconv.Atoi(s[m[6]:m[7]])
			if year < 100 {
				year += 2000
			}
		}
		d, ok := makeDate(year, time.Month(mo), day, now)
		consider(m[0], d, ok)
	}
	if best == nil {
		return nil
	}
	return &best.date
}

func makeDate(year int, month time.Month, day int, now Date) (Date, bool) {
	if month < time.January || month > time.December || day < 1 {
		return Date{}, false
	}
	if year == 0 {
		start := now.Year
		if now.Month < time.July {
			start--
		}
		year = start
		if month < time.July {
			year++
		}
	}
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day { // e.g. February 30 rolled over
		return Date{}, false
	}
	return Date{year, month, day}, true
}
