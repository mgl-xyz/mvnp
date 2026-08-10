package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"mvnp/internal/npm"
)

func resolveRuntimeConfig(target string, overrides npm.SettingsOverrides) (npm.ResolvedSettings, string, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return npm.ResolvedSettings{}, "", err
	}
	resolved, err := npm.ResolveSettings(absTarget, overrides)
	if err != nil {
		return npm.ResolvedSettings{}, "", err
	}
	return resolved, absTarget, nil
}

func registryForProject(resolved npm.ResolvedSettings, absTarget string) npm.VersionLister {
	cacheDir, err := resolved.AbsPath(absTarget, resolved.MetadataCacheDir)
	if err != nil {
		cacheDir = filepath.Join(absTarget, npm.ProjectSettingsDir, "cache/registry")
	}
	return npm.NewCachingRegistry(resolved.Registry, cacheDir)
}

func backupDirForProject(resolved npm.ResolvedSettings, absTarget, override string) (string, error) {
	if override != "" {
		return npm.ResolveBackupDir(absTarget, override)
	}
	return resolved.AbsPath(absTarget, resolved.BackupDir)
}

func printResolvedSettings(resolved npm.ResolvedSettings) {
	fmt.Printf("registry: %s\n", resolved.Registry)
	fmt.Printf("backupDir: %s\n", resolved.BackupDir)
	fmt.Printf("autoBackup: %t\n", resolved.AutoBackup)
	fmt.Printf("backupKeepCount: %d\n", resolved.BackupKeepCount)
	fmt.Printf("metadataCacheDir: %s\n", resolved.MetadataCacheDir)
	fmt.Printf("target: %s\n", resolved.Target)
	if len(resolved.DepSections) > 0 {
		fmt.Printf("depSections: %s\n", joinList(resolved.DepSections))
	}
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

func parsePackageFlags(includeRaw, ignoreRaw string) (npm.SettingsOverrides, error) {
	include, err := npm.ParsePackageList(includeRaw)
	if err != nil {
		return npm.SettingsOverrides{}, err
	}
	ignore, err := npm.ParsePackageList(ignoreRaw)
	if err != nil {
		return npm.SettingsOverrides{}, err
	}
	return npm.SettingsOverrides{
		Include: include,
		Ignore:  ignore,
	}, nil
}

func promptIgnoredPackages(target string, recursive bool, packagePath string, sections []string) ([]string, error) {
	entries, err := npm.ListPackages(npm.ListPackagesRequest{
		Root:        target,
		Recursive:   recursive,
		PackagePath: packagePath,
		DepSections: sections,
	})
	if err != nil {
		return nil, err
	}
	unique := npm.UniquePackageNames(entries)
	if len(unique) == 0 {
		return nil, fmt.Errorf("no packages to ignore")
	}
	fmt.Fprintln(os.Stderr, "Select packages to ignore:")
	return npm.PromptSelectPackages(os.Stderr, os.Stdin, unique)
}

func promptIncludedPackages(target string, recursive bool, packagePath string, sections []string) ([]string, error) {
	entries, err := npm.ListPackages(npm.ListPackagesRequest{
		Root:        target,
		Recursive:   recursive,
		PackagePath: packagePath,
		DepSections: sections,
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no packages found")
	}
	fmt.Fprintln(os.Stderr, "Select packages to upgrade:")
	return npm.PromptSelectPackages(os.Stderr, os.Stdin, entries)
}
