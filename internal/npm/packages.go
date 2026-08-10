package npm

import (
	"fmt"
	"sort"
	"strings"
)

type PackageEntry struct {
	Name            string
	Section         string
	Version         string
	ResolvedVersion string
	PackagePath     string
}

type ListPackagesRequest struct {
	Root        string
	Recursive   bool
	PackagePath string
	DepSections []string
}

func ListPackages(request ListPackagesRequest) ([]PackageEntry, error) {
	var packageFiles []string
	var err error

	if strings.TrimSpace(request.PackagePath) != "" {
		packageFiles = []string{request.PackagePath}
	} else {
		packageFiles, err = FindPackageFiles(request.Root, request.Recursive)
		if err != nil {
			return nil, err
		}
	}

	sections := request.DepSections
	if len(sections) == 0 {
		sections = dependencySections
	}

	seen := make(map[string]PackageEntry)
	for _, path := range packageFiles {
		pkg, err := ParsePackageFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		deps, err := pkg.UpgradeableDependencies(sections, "")
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			key := dep.Name + "|" + dep.Section + "|" + path
			if _, ok := seen[key]; ok {
				continue
			}
			resolved, _ := ResolveSpecVersion(dep.Version)
			seen[key] = PackageEntry{
				Name:            dep.Name,
				Section:         dep.Section,
				Version:         dep.Version,
				ResolvedVersion: resolved,
				PackagePath:     path,
			}
		}
	}

	entries := make([]PackageEntry, 0, len(seen))
	for _, entry := range seen {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			if entries[i].Section == entries[j].Section {
				return entries[i].PackagePath < entries[j].PackagePath
			}
			return entries[i].Section < entries[j].Section
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func UniquePackageNames(entries []PackageEntry) []PackageEntry {
	seen := make(map[string]bool)
	var unique []PackageEntry
	for _, entry := range entries {
		if seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
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
		b.WriteString(fmt.Sprintf("%s%-40s %-22s %s\n",
			prefix,
			entry.Name,
			entry.Section,
			version,
		))
	}
	b.WriteString(fmt.Sprintf("\n%d package(s)\n", len(entries)))
	return b.String()
}
