package maven

import (
	"regexp"
	"strings"
)

var (
	snapshotSuffix = regexp.MustCompile(`(?i)-SNAPSHOT$`)
	preRelease     = regexp.MustCompile(`(?i)[.-](alpha|beta|rc|cr|m|ea|preview|dev)([.-]?\d*)?$`)
	calendarVer    = regexp.MustCompile(`\d{4}\.\d{2}\.\d{2}`)
)

// VersionKind classifies a Maven version string.
type VersionKind int

const (
	VersionUnknown VersionKind = iota
	VersionRelease
	VersionSnapshot
	VersionPreRelease
	VersionCalendar
)

func ClassifyVersion(version string) VersionKind {
	v := strings.TrimSpace(version)
	if v == "" || strings.HasPrefix(v, "${") || strings.HasPrefix(v, "[") || strings.HasPrefix(v, "(") {
		return VersionUnknown
	}
	if snapshotSuffix.MatchString(v) {
		return VersionSnapshot
	}
	if calendarVer.MatchString(v) {
		return VersionCalendar
	}
	if preRelease.MatchString(v) {
		return VersionPreRelease
	}
	return VersionRelease
}

func IsSnapshot(version string) bool {
	return ClassifyVersion(version) == VersionSnapshot
}

func IsRelease(version string) bool {
	k := ClassifyVersion(version)
	return k == VersionRelease || k == VersionCalendar
}

// CompareVersions compares two Maven versions.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareVersions(a, b string) int {
	return compareTokens(tokenize(a), tokenize(b))
}

func tokenize(version string) []interface{} {
	version = strings.TrimSpace(version)
	version = snapshotSuffix.ReplaceAllString(version, "")

	var tokens []interface{}
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		part := current.String()
		current.Reset()
		if num, ok := parseLeadingInt(part); ok {
			tokens = append(tokens, num)
			if rest := part[len(intString(num)):]; rest != "" {
				tokens = append(tokens, rest)
			}
			return
		}
		tokens = append(tokens, part)
	}

	for _, r := range version {
		switch r {
		case '.', '-', '_':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func parseLeadingInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n := 0
	for _, ch := range s[:i] {
		n = n*10 + int(ch-'0')
	}
	return n, true
}

func compareTokens(a, b []interface{}) int {
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		switch av := a[i].(type) {
		case int:
			bv, ok := b[i].(int)
			if !ok {
				return 1
			}
			if av < bv {
				return -1
			}
			if av > bv {
				return 1
			}
		default:
			as := av.(string)
			bs, ok := b[i].(string)
			if !ok {
				return -1
			}
			if c := strings.Compare(as, bs); c != 0 {
				return c
			}
		}
	}
	return 0
}
