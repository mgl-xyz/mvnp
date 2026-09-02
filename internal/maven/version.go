package maven

import (
	"regexp"
	"strings"
)

var (
	snapshotSuffix     = regexp.MustCompile(`(?i)-SNAPSHOT$`)
	preRelease         = regexp.MustCompile(`(?i)[.-](alpha|beta|rc|cr|m|ea|preview|dev)([.-]?\d*)?$`)
	calendarVer        = regexp.MustCompile(`\d{4}\.\d{2}\.\d{2}`)
	timestampQualifier = regexp.MustCompile(`(?i)^v\d{8}`)
	releaseQualifierSP = regexp.MustCompile(`(?i)^sp\d*$`)
	coreAndQualifier   = regexp.MustCompile(`(?i)^(\d+(?:[._-]\d+)*)(?:[._-](.+))?$`)
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

// VersionFamily returns the platform/variant qualifier of a Maven version.
// Canonical releases like 2.0.64 and 4.1.108.Final have an empty family.
// Device/runtime forks keep their suffix: 2.0.64.android8 → "android8", 32.1.3-jre → "jre".
func VersionFamily(version string) string {
	v := snapshotSuffix.ReplaceAllString(strings.TrimSpace(version), "")
	v = preRelease.ReplaceAllString(v, "")
	m := coreAndQualifier.FindStringSubmatch(v)
	if len(m) < 3 || m[2] == "" {
		return ""
	}
	first := strings.ToLower(qualifierHead(m[2]))
	if first == "" || isCanonicalQualifier(first) {
		return ""
	}
	return first
}

func qualifierHead(qual string) string {
	qual = strings.TrimSpace(qual)
	for i, r := range qual {
		if r == '.' || r == '-' || r == '_' {
			return qual[:i]
		}
	}
	return qual
}

func isGAEquivalent(s string) bool {
	switch s {
	case "final", "ga", "release":
		return true
	}
	return false
}

func isPostReleaseQualifier(s string) bool {
	return releaseQualifierSP.MatchString(s)
}

func isCanonicalQualifier(s string) bool {
	return isGAEquivalent(s) || isPostReleaseQualifier(s) || timestampQualifier.MatchString(s)
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
			return -cmpExtra(b[i:])
		}
		if i >= len(b) {
			return cmpExtra(a[i:])
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

// cmpExtra ranks leftover tokens after a shared numeric prefix.
// Extra numbers (1.0.1) and service packs (1.0-sp1) make a version newer.
// Unknown qualifiers (2.0.64.android8) and pre-releases are older than the prefix.
func cmpExtra(tokens []interface{}) int {
	for _, tok := range tokens {
		switch v := tok.(type) {
		case int:
			if v == 0 {
				continue
			}
			return 1
		case string:
			lower := strings.ToLower(v)
			if isPostReleaseQualifier(lower) {
				return 1
			}
			if isGAEquivalent(lower) || timestampQualifier.MatchString(lower) {
				continue
			}
			return -1
		}
	}
	return 0
}
