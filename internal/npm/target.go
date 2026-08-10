package npm

import (
	"fmt"
	"strings"
)

// Target selects how registry versions are chosen (aligned with npm-check-updates).
type Target string

const (
	TargetLatest   Target = "latest"
	TargetGreatest Target = "greatest"
	TargetMinor    Target = "minor"
	TargetPatch    Target = "patch"
	TargetSemver   Target = "semver"
)

func ParseTarget(raw string) (Target, error) {
	switch Target(strings.ToLower(strings.TrimSpace(raw))) {
	case TargetLatest, "release", "stable":
		return TargetLatest, nil
	case TargetGreatest, "newest":
		return TargetGreatest, nil
	case TargetMinor:
		return TargetMinor, nil
	case TargetPatch:
		return TargetPatch, nil
	case TargetSemver:
		return TargetSemver, nil
	default:
		return "", fmt.Errorf("unknown target %q (supported: latest, greatest, minor, patch, semver)", raw)
	}
}

// TargetOptions controls version filtering during selection.
type TargetOptions struct {
	IncludePrerelease bool
	AllowMajor        bool
	AllowMinor        bool
	AllowPatch        bool
	AllowDowngrade    bool
}

func DefaultTargetOptions(target Target) TargetOptions {
	opts := TargetOptions{
		AllowMajor: true,
		AllowMinor: true,
		AllowPatch: true,
	}
	if target == TargetGreatest {
		opts.IncludePrerelease = true
	}
	return opts
}

// SelectTargetVersion picks a registry version for the current resolved version.
func SelectTargetVersion(current string, available []string, target Target, opts TargetOptions) (string, bool) {
	if len(available) == 0 {
		return "", false
	}

	candidates := filterAvailable(available, opts)
	if len(candidates) == 0 {
		return "", false
	}

	switch target {
	case TargetLatest:
		stable := filterStable(candidates)
		if len(stable) == 0 {
			stable = candidates
		}
		chosen := pickLatest(stable)
		return chosen, upgradeAllowed(current, chosen, opts)
	case TargetGreatest:
		chosen := pickLatest(candidates)
		return chosen, upgradeAllowed(current, chosen, opts)
	case TargetMinor:
		chosen := pickWithinMinor(current, candidates)
		if chosen == "" {
			return "", false
		}
		return chosen, upgradeAllowed(current, chosen, opts)
	case TargetPatch:
		chosen := pickWithinPatch(current, candidates)
		if chosen == "" {
			return "", false
		}
		return chosen, upgradeAllowed(current, chosen, opts)
	case TargetSemver:
		chosen := pickWithinSemverRange(current, candidates)
		if chosen == "" {
			return "", false
		}
		return chosen, CompareSemver(chosen, current) > 0
	default:
		return "", false
	}
}

func filterAvailable(versions []string, opts TargetOptions) []string {
	var out []string
	for _, v := range versions {
		if v == "" {
			continue
		}
		if !opts.IncludePrerelease && IsPrerelease(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func filterStable(versions []string) []string {
	var out []string
	for _, v := range versions {
		if !IsPrerelease(v) {
			out = append(out, v)
		}
	}
	return out
}

func upgradeAllowed(current, target string, opts TargetOptions) bool {
	if current == target {
		return false
	}
	cmp := CompareSemver(target, current)
	if cmp > 0 {
		return segmentAllowed(current, target, opts)
	}
	if cmp < 0 {
		return opts.AllowDowngrade
	}
	return false
}

func segmentAllowed(current, target string, opts TargetOptions) bool {
	if opts.AllowMajor && opts.AllowMinor && opts.AllowPatch {
		return true
	}
	c := numericParts(current)
	t := numericParts(target)
	for len(c) < 3 {
		c = append(c, 0)
	}
	for len(t) < 3 {
		t = append(t, 0)
	}
	if t[0] != c[0] {
		return opts.AllowMajor
	}
	if t[1] != c[1] {
		return opts.AllowMinor
	}
	if t[2] != c[2] {
		return opts.AllowPatch
	}
	return true
}

func numericParts(version string) []int {
	v, ok := ResolveSpecVersion(version)
	if !ok {
		v = version
	}
	parts := strings.Split(v, ".")
	var nums []int
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			nums = append(nums, 0)
			continue
		}
		n := 0
		fmt.Sscanf(parts[i], "%d", &n)
		nums = append(nums, n)
	}
	return nums
}

func pickWithinMinor(current string, versions []string) string {
	c := numericParts(current)
	var best string
	for _, v := range versions {
		t := numericParts(v)
		if t[0] != c[0] {
			continue
		}
		if CompareSemver(v, current) <= 0 {
			continue
		}
		if best == "" || CompareSemver(v, best) > 0 {
			best = v
		}
	}
	return best
}

func pickWithinPatch(current string, versions []string) string {
	c := numericParts(current)
	var best string
	for _, v := range versions {
		t := numericParts(v)
		if t[0] != c[0] || t[1] != c[1] {
			continue
		}
		if CompareSemver(v, current) <= 0 {
			continue
		}
		if best == "" || CompareSemver(v, best) > 0 {
			best = v
		}
	}
	return best
}

func pickWithinSemverRange(currentSpec string, versions []string) string {
	resolved, ok := ResolveSpecVersion(currentSpec)
	if !ok {
		return ""
	}
	var best string
	for _, v := range versions {
		if CompareSemver(v, resolved) <= 0 {
			continue
		}
		if !matchesSemverPolicy(currentSpec, v) {
			continue
		}
		if best == "" || CompareSemver(v, best) > 0 {
			best = v
		}
	}
	return best
}

func matchesSemverPolicy(spec, version string) bool {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "^") {
		base, ok := ResolveSpecVersion(spec)
		if !ok {
			return true
		}
		b := numericParts(base)
		v := numericParts(version)
		if v[0] != b[0] {
			return false
		}
		return CompareSemver(version, base) >= 0
	}
	if strings.HasPrefix(spec, "~") {
		base, ok := ResolveSpecVersion(spec)
		if !ok {
			return true
		}
		b := numericParts(base)
		v := numericParts(version)
		return v[0] == b[0] && v[1] == b[1] && CompareSemver(version, base) >= 0
	}
	return true
}
