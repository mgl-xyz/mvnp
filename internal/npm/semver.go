package npm

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	prereleaseSuffix = regexp.MustCompile(`(?i)[.-](alpha|beta|rc|pre|dev|snapshot|next)([.-]?\d*)?$`)
	leadingV         = regexp.MustCompile(`(?i)^v`)
)

// CompareSemver compares two semver-like version strings.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareSemver(a, b string) int {
	aMain, aPre := splitPrerelease(a)
	bMain, bPre := splitPrerelease(b)
	if cmp := compareTokens(tokenizeMain(aMain), tokenizeMain(bMain)); cmp != 0 {
		return cmp
	}
	if aPre == "" && bPre == "" {
		return 0
	}
	if aPre == "" {
		return 1
	}
	if bPre == "" {
		return -1
	}
	if aPre < bPre {
		return -1
	}
	if aPre > bPre {
		return 1
	}
	return 0
}

func splitPrerelease(version string) (string, string) {
	version = strings.TrimSpace(version)
	version = leadingV.ReplaceAllString(version, "")
	version = strings.SplitN(version, "+", 2)[0]
	if idx := strings.Index(version, "-"); idx >= 0 {
		return version[:idx], version[idx+1:]
	}
	return version, ""
}

func tokenizeMain(version string) []interface{} {
	var tokens []interface{}
	for _, part := range strings.Split(version, ".") {
		if part == "" {
			tokens = append(tokens, 0)
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			tokens = append(tokens, n)
			continue
		}
		tokens = append(tokens, part)
	}
	return tokens
}

func tokenizeSemver(version string) []interface{} {
	main, _ := splitPrerelease(version)
	return tokenizeMain(main)
}

func compareTokens(a, b []interface{}) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv interface{}
		if i < len(a) {
			av = a[i]
		} else {
			av = 0
		}
		if i < len(b) {
			bv = b[i]
		} else {
			bv = 0
		}
		switch avTyped := av.(type) {
		case int:
			bvInt, ok := bv.(int)
			if !ok {
				if _, ok := bv.(string); ok {
					return -1
				}
				return 1
			}
			if avTyped < bvInt {
				return -1
			}
			if avTyped > bvInt {
				return 1
			}
		case string:
			bvStr, ok := bv.(string)
			if !ok {
				return 1
			}
			if avTyped < bvStr {
				return -1
			}
			if avTyped > bvStr {
				return 1
			}
		}
	}
	return 0
}

func IsPrerelease(version string) bool {
	v := strings.TrimSpace(version)
	v = leadingV.ReplaceAllString(v, "")
	if idx := strings.IndexAny(v, "-"); idx >= 0 {
		return true
	}
	return prereleaseSuffix.MatchString(v)
}

func IsRegistryVersionSpec(spec string) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "*" {
		return true
	}
	lower := strings.ToLower(spec)
	for _, prefix := range []string{"workspace:", "file:", "link:", "git:", "github:", "http:", "https:", "npm:"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

// ResolveSpecVersion extracts a concrete version from a package.json range for comparison.
func ResolveSpecVersion(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	if spec == "*" {
		return "0.0.0", true
	}

	spec = leadingV.ReplaceAllString(spec, "")

	for _, prefix := range []string{"^", "~", ">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(spec, prefix) {
			rest := strings.TrimSpace(strings.TrimPrefix(spec, prefix))
			if v, ok := parseConcreteVersion(rest); ok {
				return v, true
			}
		}
	}

	if strings.HasSuffix(spec, ".x") || strings.HasSuffix(spec, ".X") {
		base := strings.TrimSuffix(strings.TrimSuffix(spec, ".x"), ".X")
		parts := strings.Split(base, ".")
		for len(parts) < 3 {
			parts = append(parts, "0")
		}
		return strings.Join(parts, "."), true
	}

	if v, ok := parseConcreteVersion(spec); ok {
		return v, true
	}
	return "", false
}

func parseConcreteVersion(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	raw = leadingV.ReplaceAllString(raw, "")
	if raw == "" {
		return "", false
	}
	parts := strings.SplitN(raw, "+", 2)[0]
	parts = strings.SplitN(parts, "-", 2)[0]
	segments := strings.Split(parts, ".")
	if len(segments) == 0 {
		return "", false
	}
	for _, seg := range segments {
		if seg == "" {
			return "", false
		}
		if _, err := strconv.Atoi(seg); err != nil {
			if len(segments) == 1 {
				return parts, true
			}
			return "", false
		}
	}
	for len(segments) < 3 {
		segments = append(segments, "0")
	}
	return strings.Join(segments[:3], "."), true
}

// UpgradeSpec rewrites a package.json version spec to targetVersion while preserving range style.
func UpgradeSpec(spec, targetVersion string) string {
	spec = strings.TrimSpace(spec)
	targetVersion = strings.TrimSpace(targetVersion)
	targetVersion = leadingV.ReplaceAllString(targetVersion, "")

	if spec == "" || spec == "*" {
		return spec
	}
	if strings.HasPrefix(spec, "v") && !strings.ContainsAny(spec, "^~><=") {
		return "v" + targetVersion
	}

	switch {
	case strings.HasPrefix(spec, "^"):
		return "^" + targetVersion
	case strings.HasPrefix(spec, "~"):
		return "~" + targetVersion
	case strings.HasPrefix(spec, ">="):
		return ">=" + targetVersion
	case strings.HasPrefix(spec, "<="):
		return "<=" + targetVersion
	case strings.HasPrefix(spec, ">"):
		return ">" + targetVersion
	case strings.HasPrefix(spec, "<"):
		return "<" + targetVersion
	case strings.HasPrefix(spec, "="):
		return "=" + targetVersion
	case strings.HasSuffix(spec, ".x") || strings.HasSuffix(spec, ".X"):
		major := strings.Split(strings.TrimSuffix(strings.TrimSuffix(spec, ".x"), ".X"), ".")[0]
		if major == "" {
			return targetVersion
		}
		targetMajor := strings.Split(targetVersion, ".")[0]
		return targetMajor + ".x"
	default:
		return targetVersion
	}
}

func pickLatest(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if CompareSemver(v, latest) > 0 {
			latest = v
		}
	}
	return latest
}

func pickLatestStable(versions []string) string {
	var stable []string
	for _, v := range versions {
		if !IsPrerelease(v) {
			stable = append(stable, v)
		}
	}
	if len(stable) == 0 {
		return pickLatest(versions)
	}
	return pickLatest(stable)
}
