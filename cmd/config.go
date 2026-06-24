package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"mvnp/internal/maven"
)

func resolveRuntimeConfig(target string, overrides maven.SettingsOverrides) (maven.ResolvedSettings, string, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return maven.ResolvedSettings{}, "", err
	}
	resolved, err := maven.ResolveSettings(absTarget, overrides)
	if err != nil {
		return maven.ResolvedSettings{}, "", err
	}
	return resolved, absTarget, nil
}

func repositoryForProject(resolved maven.ResolvedSettings, absTarget string) maven.VersionLister {
	cacheDir, err := resolved.AbsPath(absTarget, resolved.MetadataCacheDir)
	if err != nil {
		cacheDir = filepath.Join(absTarget, maven.ProjectSettingsDir, "cache/metadata")
	}
	return maven.NewCachingRepository(resolved.Repository, cacheDir)
}

func backupDirForProject(resolved maven.ResolvedSettings, absTarget, override string) (string, error) {
	if override != "" {
		return maven.ResolveBackupDir(absTarget, override)
	}
	return resolved.AbsPath(absTarget, resolved.BackupDir)
}

func printResolvedSettings(resolved maven.ResolvedSettings) {
	fmt.Printf("repository: %s\n", resolved.Repository)
	fmt.Printf("backupDir: %s\n", resolved.BackupDir)
	fmt.Printf("autoBackup: %t\n", resolved.AutoBackup)
	fmt.Printf("backupKeepCount: %d\n", resolved.BackupKeepCount)
	fmt.Printf("metadataCacheDir: %s\n", resolved.MetadataCacheDir)
	fmt.Printf("policy: %s\n", resolved.Policy)
	if len(resolved.Ignore) > 0 {
		fmt.Printf("ignore: %s\n", joinList(resolved.Ignore))
	}
	if len(resolved.Include) > 0 {
		fmt.Printf("include: %s\n", joinList(resolved.Include))
	}
	if resolved.GlobalPath != "" {
		fmt.Printf("globalSettings: %s\n", resolved.GlobalPath)
	}
	if resolved.ProjectPath != "" {
		fmt.Printf("projectSettings: %s\n", resolved.ProjectPath)
	}
}

func joinList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := items[0]
	for i := 1; i < len(items); i++ {
		out += ", " + items[i]
	}
	return out
}

func parseCoordinateFlags(includeRaw, ignoreRaw string) (maven.SettingsOverrides, error) {
	include, err := maven.ParseCoordinateList(includeRaw)
	if err != nil {
		return maven.SettingsOverrides{}, err
	}
	ignore, err := maven.ParseCoordinateList(ignoreRaw)
	if err != nil {
		return maven.SettingsOverrides{}, err
	}
	return maven.SettingsOverrides{
		Include: include,
		Ignore:  ignore,
	}, nil
}

func promptIgnoredPackages(target string, recursive, includeParent bool, pomPath string) ([]string, error) {
	entries, err := maven.ListPackages(maven.ListPackagesRequest{
		Root:          target,
		Recursive:     recursive,
		IncludeParent: includeParent,
		POMPath:       pomPath,
	})
	if err != nil {
		return nil, err
	}
	unique := maven.UniquePackageCoordinates(entries)
	if len(unique) == 0 {
		return nil, fmt.Errorf("no packages to ignore")
	}
	fmt.Fprintln(os.Stderr, "Select packages to ignore:")
	return maven.PromptSelectPackages(os.Stderr, os.Stdin, unique)
}

func promptIncludedPackages(target string, recursive, includeParent bool, pomPath string) ([]string, error) {
	entries, err := maven.ListPackages(maven.ListPackagesRequest{
		Root:          target,
		Recursive:     recursive,
		IncludeParent: includeParent,
		POMPath:       pomPath,
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no packages found")
	}
	fmt.Fprintln(os.Stderr, "Select packages to upgrade:")
	return maven.PromptSelectPackages(os.Stderr, os.Stdin, entries)
}
