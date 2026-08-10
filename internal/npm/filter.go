package npm

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PackageFilter controls which package names are processed.
type PackageFilter struct {
	include []string
	ignore  []string
}

func NewPackageFilter(include, ignore []string) (PackageFilter, error) {
	filter := PackageFilter{
		include: normalizePatterns(include),
		ignore:  normalizePatterns(ignore),
	}
	for _, pattern := range filter.include {
		if err := validatePattern(pattern); err != nil {
			return PackageFilter{}, fmt.Errorf("include %w", err)
		}
	}
	for _, pattern := range filter.ignore {
		if err := validatePattern(pattern); err != nil {
			return PackageFilter{}, fmt.Errorf("ignore %w", err)
		}
	}
	return filter, nil
}

func (f PackageFilter) HasInclude() bool {
	return len(f.include) > 0
}

func (f PackageFilter) Allows(name string) (bool, string) {
	if matchesAny(name, f.ignore) {
		return false, "ignored"
	}
	if f.HasInclude() && !matchesAny(name, f.include) {
		return false, "not selected"
	}
	return true, ""
}

func normalizePatterns(items []string) []string {
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func validatePattern(pattern string) error {
	if pattern == "" {
		return nil
	}
	return nil
}

func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(name, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(name, pattern string) bool {
	if pattern == name {
		return true
	}
	if strings.Contains(pattern, "*") {
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
		// Also support simple prefix* matching for scoped packages.
		if strings.HasSuffix(pattern, "*") {
			return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
		}
	}
	return false
}

func ParsePackageList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	var names []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		names = append(names, part)
	}
	return names, nil
}

func MergePackageLists(base, extra []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, list := range [][]string{base, extra} {
		for _, item := range list {
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}
