package maven

import (
	"fmt"
	"strings"
)

// CoordinateFilter controls which groupId:artifactId coordinates are processed.
type CoordinateFilter struct {
	include map[string]struct{}
	ignore  map[string]struct{}
}

func NewCoordinateFilter(include, ignore []string) (CoordinateFilter, error) {
	filter := CoordinateFilter{
		include: make(map[string]struct{}),
		ignore:  make(map[string]struct{}),
	}
	for _, raw := range include {
		coord, err := normalizeCoordinate(raw)
		if err != nil {
			return CoordinateFilter{}, fmt.Errorf("include %w", err)
		}
		if coord != "" {
			filter.include[coord] = struct{}{}
		}
	}
	for _, raw := range ignore {
		coord, err := normalizeCoordinate(raw)
		if err != nil {
			return CoordinateFilter{}, fmt.Errorf("ignore %w", err)
		}
		if coord != "" {
			filter.ignore[coord] = struct{}{}
		}
	}
	return filter, nil
}

func (f CoordinateFilter) HasInclude() bool {
	return len(f.include) > 0
}

func (f CoordinateFilter) Allows(groupID, artifactID string) (bool, string) {
	coord := ArtifactCoordinate(groupID, artifactID)
	if _, ok := f.ignore[coord]; ok {
		return false, "ignored"
	}
	if f.HasInclude() {
		if _, ok := f.include[coord]; !ok {
			return false, "not selected"
		}
	}
	return true, ""
}

func normalizeCoordinate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	groupID, artifactID, err := ParseArtifactCoordinate(raw)
	if err != nil {
		return "", err
	}
	return ArtifactCoordinate(groupID, artifactID), nil
}

func ParseCoordinateList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	var coords []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		coord, err := normalizeCoordinate(part)
		if err != nil {
			return nil, err
		}
		coords = append(coords, coord)
	}
	return coords, nil
}

func MergeCoordinateLists(base, extra []string) []string {
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
