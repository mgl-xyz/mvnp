package maven

import (
	"fmt"
	"sort"
	"strings"
)

// PackageEntry describes one upgradeable coordinate found in pom.xml.
type PackageEntry struct {
	GroupID         string
	ArtifactID      string
	Coordinate      string
	Section         string
	Version         string
	ResolvedVersion string
	POMPath         string
}

// ListPackagesRequest configures package listing.
type ListPackagesRequest struct {
	Root          string
	Recursive     bool
	IncludeParent bool
	POMPath       string
}

func ListPackages(request ListPackagesRequest) ([]PackageEntry, error) {
	var pomFiles []string
	var err error

	if strings.TrimSpace(request.POMPath) != "" {
		pomFiles = []string{request.POMPath}
	} else {
		pomFiles, err = FindPOMFiles(request.Root, request.Recursive)
		if err != nil {
			return nil, err
		}
	}

	seen := make(map[string]PackageEntry)
	for _, pomPath := range pomFiles {
		pom, err := ParsePOM(pomPath)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", pomPath, err)
		}
		for _, dep := range pom.UpgradeableDependencies(request.IncludeParent, "") {
			coord := ArtifactCoordinate(dep.GroupID, dep.ArtifactID)
			key := coord + "|" + dep.Section + "|" + pomPath
			if _, ok := seen[key]; ok {
				continue
			}
			resolved := pom.ResolveVersion(dep.Version)
			seen[key] = PackageEntry{
				GroupID:         dep.GroupID,
				ArtifactID:      dep.ArtifactID,
				Coordinate:      coord,
				Section:         dep.Section,
				Version:         dep.Version,
				ResolvedVersion: resolved,
				POMPath:         pomPath,
			}
		}
	}

	entries := make([]PackageEntry, 0, len(seen))
	for _, entry := range seen {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Coordinate == entries[j].Coordinate {
			if entries[i].Section == entries[j].Section {
				return entries[i].POMPath < entries[j].POMPath
			}
			return entries[i].Section < entries[j].Section
		}
		return entries[i].Coordinate < entries[j].Coordinate
	})
	return entries, nil
}

func UniquePackageCoordinates(entries []PackageEntry) []PackageEntry {
	seen := make(map[string]bool)
	var unique []PackageEntry
	for _, entry := range entries {
		if seen[entry.Coordinate] {
			continue
		}
		seen[entry.Coordinate] = true
		unique = append(unique, entry)
	}
	return unique
}

func FormatPackageList(entries []PackageEntry, numbered bool) string {
	if len(entries) == 0 {
		return "no packages found\n"
	}
	var b strings.Builder
	for index, entry := range entries {
		prefix := ""
		if numbered {
			prefix = fmt.Sprintf("%3d  ", index+1)
		}
		version := entry.Version
		if entry.ResolvedVersion != "" && entry.ResolvedVersion != entry.Version {
			version = fmt.Sprintf("%s [%s]", entry.Version, entry.ResolvedVersion)
		}
		b.WriteString(fmt.Sprintf("%s%-56s %-22s %s\n",
			prefix,
			entry.Coordinate,
			entry.Section,
			version,
		))
	}
	b.WriteString(fmt.Sprintf("\n%d package(s)\n", len(entries)))
	return b.String()
}

func FilterWorkItems(items []upgradeWorkItem, filter CoordinateFilter) []upgradeWorkItem {
	if !filter.HasInclude() && len(filter.ignore) == 0 {
		return items
	}
	var filtered []upgradeWorkItem
	for _, item := range items {
		allowed, _ := filter.Allows(item.dep.GroupID, item.dep.ArtifactID)
		if allowed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
