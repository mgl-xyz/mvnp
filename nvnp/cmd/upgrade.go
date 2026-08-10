package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"mvnp/internal/npm"
)

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	targetName := fs.String("target", "", "upgrade target: latest, greatest, minor, patch, semver")
	recursive := fs.Bool("recursive", false, "process child package.json files")
	dryRun := fs.Bool("dry-run", false, "preview changes without writing package.json")
	only := fs.String("only", "", "only upgrade package name")
	registry := fs.String("registry", "", "npm registry base URL")
	includePre := fs.Bool("include-pre-releases", false, "include alpha/beta/rc versions")
	allowMajor := fs.Bool("allow-major", true, "allow major version upgrades")
	allowMinor := fs.Bool("allow-minor", true, "allow minor version upgrades")
	allowPatch := fs.Bool("allow-patch", true, "allow patch version upgrades")
	allowDowngrade := fs.Bool("allow-downgrade", false, "allow downgrading versions")
	noBackup := fs.Bool("no-backup", false, "skip automatic backup before upgrade")
	backupDir := fs.String("backup-dir", "", "backup storage directory")
	quiet := fs.Bool("quiet", false, "disable progress output")
	verbose := fs.Bool("verbose", false, "show detailed progress output")
	summary := fs.Bool("summary", false, "print final summary report to stdout")
	includeRaw := fs.String("include", "", "only upgrade package name list")
	ignoreRaw := fs.String("ignore", "", "skip package name list")
	selectUpgrade := fs.Bool("select", false, "interactively choose packages to upgrade")
	ignoreSelect := fs.Bool("ignore-select", false, "interactively choose packages to ignore")
	packagePath := fs.String("package", "", "use a specific package.json for list/select")
	depRaw := fs.String("dep", "", "dependency sections: prod,dev,peer,optional (comma-separated)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	targetDir := "."
	if fs.NArg() > 0 {
		targetDir = fs.Arg(0)
	}

	flagOverrides, err := parsePackageFlags(*includeRaw, *ignoreRaw)
	if err != nil {
		return err
	}
	flagOverrides.Registry = *registry
	flagOverrides.BackupDir = *backupDir
	flagOverrides.Target = *targetName

	resolved, absTarget, err := resolveRuntimeConfig(targetDir, flagOverrides)
	if err != nil {
		return err
	}

	depSections, err := parseDepSections(*depRaw, resolved.DepSections)
	if err != nil {
		return err
	}

	if *selectUpgrade {
		selected, err := promptIncludedPackages(targetDir, *recursive, *packagePath, depSections)
		if err != nil {
			return err
		}
		flagOverrides.Include = npm.MergePackageLists(flagOverrides.Include, selected)
	}
	if *ignoreSelect {
		ignored, err := promptIgnoredPackages(targetDir, *recursive, *packagePath, depSections)
		if err != nil {
			return err
		}
		flagOverrides.Ignore = npm.MergePackageLists(flagOverrides.Ignore, ignored)
	}

	resolved, absTarget, err = resolveRuntimeConfig(targetDir, flagOverrides)
	if err != nil {
		return err
	}

	targetValue := resolved.Target
	if changedFlag(fs, "target") {
		targetValue = *targetName
	}
	upgradeTarget, err := npm.ParseTarget(targetValue)
	if err != nil {
		return err
	}

	opts := npm.DefaultTargetOptions(upgradeTarget)
	if changedFlag(fs, "include-pre-releases") {
		opts.IncludePrerelease = *includePre
	}
	if changedFlag(fs, "allow-major") {
		opts.AllowMajor = *allowMajor
	}
	if changedFlag(fs, "allow-minor") {
		opts.AllowMinor = *allowMinor
	}
	if changedFlag(fs, "allow-patch") {
		opts.AllowPatch = *allowPatch
	}
	if changedFlag(fs, "allow-downgrade") {
		opts.AllowDowngrade = *allowDowngrade
	}

	filter, err := resolved.PackageFilter()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*only) != "" {
		filter, err = npm.NewPackageFilter([]string{*only}, nil)
		if err != nil {
			return err
		}
	}

	backupStore, err := backupDirForProject(resolved, absTarget, *backupDir)
	if err != nil {
		return err
	}

	autoBackup := resolved.AutoBackup
	if changedFlag(fs, "no-backup") {
		autoBackup = !*noBackup
	}

	report, err := npm.Upgrade(npm.UpgradeRequest{
		Root:            targetDir,
		Recursive:       *recursive,
		Target:          upgradeTarget,
		Options:         opts,
		Registry:        registryForProject(resolved, absTarget),
		DryRun:          *dryRun,
		OnlyPackage:     *only,
		AutoBackup:      autoBackup,
		BackupDir:       backupStore,
		BackupKeepCount: resolved.BackupKeepCount,
		Progress:        npm.DefaultProgress(*quiet, *verbose),
		Filter:          filter,
		DepSections:     depSections,
	})
	if err != nil {
		return err
	}

	if *summary || *quiet {
		fmt.Print(npm.FormatReport(report, *dryRun))
	}
	return nil
}

func parseDepSections(raw string, defaults []string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaults, nil
	}
	aliases := map[string]string{
		"prod":     "dependencies",
		"dev":      "devDependencies",
		"peer":     "peerDependencies",
		"optional": "optionalDependencies",
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})
	var sections []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if mapped, ok := aliases[part]; ok {
			sections = append(sections, mapped)
			continue
		}
		sections = append(sections, part)
	}
	if len(sections) == 0 {
		return defaults, nil
	}
	return sections, nil
}
