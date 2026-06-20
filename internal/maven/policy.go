package maven

import (
	"fmt"
	"strings"
)

// Policy mirrors the goals of Maven Versions Plugin.
type Policy string

const (
	PolicyLatestReleases  Policy = "latest-releases"
	PolicyLatestSnapshots Policy = "latest-snapshots"
	PolicyLatestVersions  Policy = "latest-versions"
	PolicyNextReleases    Policy = "next-releases"
	PolicyNextSnapshots   Policy = "next-snapshots"
	PolicyNextVersions    Policy = "next-versions"
	PolicyReleases        Policy = "releases"
)

func ParsePolicy(raw string) (Policy, error) {
	switch Policy(strings.ToLower(strings.TrimSpace(raw))) {
	case PolicyLatestReleases, "release", "stable":
		return PolicyLatestReleases, nil
	case PolicyLatestSnapshots, "snapshot":
		return PolicyLatestSnapshots, nil
	case PolicyLatestVersions, "latest":
		return PolicyLatestVersions, nil
	case PolicyNextReleases:
		return PolicyNextReleases, nil
	case PolicyNextSnapshots:
		return PolicyNextSnapshots, nil
	case PolicyNextVersions, "next":
		return PolicyNextVersions, nil
	case PolicyReleases, "use-releases":
		return PolicyReleases, nil
	default:
		return "", fmt.Errorf("unknown policy %q (supported: latest-releases, latest-snapshots, latest-versions, next-releases, next-snapshots, next-versions, releases)", raw)
	}
}

// PolicyOptions controls version filtering during selection.
type PolicyOptions struct {
	AllowSnapshots          bool
	AllowMajorUpdates       bool
	AllowMinorUpdates       bool
	AllowIncrementalUpdates bool
	AllowDowngrade          bool
	IgnorePreReleases       bool
}

func DefaultPolicyOptions(policy Policy) PolicyOptions {
	opts := PolicyOptions{
		AllowMajorUpdates:       true,
		AllowMinorUpdates:       true,
		AllowIncrementalUpdates: true,
		AllowDowngrade:          false,
		IgnorePreReleases:       true,
	}
	switch policy {
	case PolicyLatestSnapshots, PolicyNextSnapshots:
		opts.AllowSnapshots = true
	case PolicyLatestVersions, PolicyNextVersions:
		opts.AllowSnapshots = true
	default:
		opts.AllowSnapshots = false
	}
	return opts
}

// SelectVersion picks a target version from available versions for the given policy.
func SelectVersion(current string, available []string, policy Policy, opts PolicyOptions) (string, bool) {
	if len(available) == 0 {
		return "", false
	}

	candidates := filterCandidates(available, opts)
	if len(candidates) == 0 {
		return "", false
	}

	switch policy {
	case PolicyReleases:
		if !IsSnapshot(current) {
			return "", false
		}
		releases := filterByKind(candidates, func(v string) bool { return IsRelease(v) })
		if len(releases) == 0 {
			return "", false
		}
		return pickLatest(releases), true
	case PolicyLatestReleases:
		if !shouldModifyForReleasePolicy(current) {
			return "", false
		}
		releases := filterByKind(candidates, func(v string) bool { return IsRelease(v) })
		if len(releases) == 0 {
			return "", false
		}
		target := pickLatest(releases)
		return target, isUpgradeAllowed(current, target, opts)
	case PolicyLatestSnapshots:
		if !IsSnapshot(current) {
			return "", false
		}
		snapshots := filterByKind(candidates, IsSnapshot)
		if len(snapshots) == 0 {
			return "", false
		}
		target := pickLatest(snapshots)
		return target, isUpgradeAllowed(current, target, opts)
	case PolicyLatestVersions:
		target := pickLatest(candidates)
		return target, isUpgradeAllowed(current, target, opts)
	case PolicyNextReleases:
		if !shouldModifyForReleasePolicy(current) {
			return "", false
		}
		releases := filterByKind(candidates, func(v string) bool { return IsRelease(v) })
		return pickNext(current, releases, opts)
	case PolicyNextSnapshots:
		if !IsSnapshot(current) {
			return "", false
		}
		snapshots := filterByKind(candidates, IsSnapshot)
		return pickNext(current, snapshots, opts)
	case PolicyNextVersions:
		return pickNext(current, candidates, opts)
	default:
		return "", false
	}
}

func shouldModifyForReleasePolicy(current string) bool {
	k := ClassifyVersion(current)
	return k == VersionRelease || k == VersionCalendar || k == VersionPreRelease
}

func filterCandidates(versions []string, opts PolicyOptions) []string {
	var out []string
	for _, v := range versions {
		if v == "" {
			continue
		}
		if !opts.AllowSnapshots && IsSnapshot(v) {
			continue
		}
		if opts.IgnorePreReleases && ClassifyVersion(v) == VersionPreRelease {
			continue
		}
		out = append(out, v)
	}
	return out
}

func filterByKind(versions []string, pred func(string) bool) []string {
	var out []string
	for _, v := range versions {
		if pred(v) {
			out = append(out, v)
		}
	}
	return out
}

func pickLatest(versions []string) string {
	latest := versions[0]
	for _, v := range versions[1:] {
		if CompareVersions(v, latest) > 0 {
			latest = v
		}
	}
	return latest
}

func pickNext(current string, versions []string, opts PolicyOptions) (string, bool) {
	var newer []string
	for _, v := range versions {
		if CompareVersions(v, current) > 0 {
			newer = append(newer, v)
		}
	}
	if len(newer) == 0 {
		return "", false
	}
	target := pickLatest([]string{newer[0]})
	for _, v := range newer[1:] {
		if CompareVersions(v, target) < 0 {
			target = v
		}
	}
	return target, isUpgradeAllowed(current, target, opts)
}

func isUpgradeAllowed(current, target string, opts PolicyOptions) bool {
	if current == target {
		return false
	}
	cmp := CompareVersions(target, current)
	if cmp > 0 {
		return segmentUpgradeAllowed(current, target, opts)
	}
	if cmp < 0 {
		return opts.AllowDowngrade
	}
	return false
}

func segmentUpgradeAllowed(current, target string, opts PolicyOptions) bool {
	if opts.AllowMajorUpdates && opts.AllowMinorUpdates && opts.AllowIncrementalUpdates {
		return true
	}

	cParts := numericPrefix(current)
	tParts := numericPrefix(target)
	for len(cParts) < 3 {
		cParts = append(cParts, 0)
	}
	for len(tParts) < 3 {
		tParts = append(tParts, 0)
	}

	if tParts[0] != cParts[0] {
		return opts.AllowMajorUpdates
	}
	if tParts[1] != cParts[1] {
		return opts.AllowMinorUpdates
	}
	if tParts[2] != cParts[2] {
		return opts.AllowIncrementalUpdates
	}
	return true
}

func numericPrefix(version string) []int {
	version = snapshotSuffix.ReplaceAllString(strings.TrimSpace(version), "")
	parts := strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	var nums []int
	for _, part := range parts {
		n, ok := parseLeadingInt(part)
		if !ok {
			break
		}
		nums = append(nums, n)
	}
	return nums
}
