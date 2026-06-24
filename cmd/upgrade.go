package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"mvnp/internal/maven"
)

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	policyName := fs.String("policy", "", "version upgrade policy")
	recursive := fs.Bool("recursive", false, "process child module pom.xml files")
	dryRun := fs.Bool("dry-run", false, "preview changes without writing pom.xml")
	includeParent := fs.Bool("include-parent", false, "also upgrade parent version")
	only := fs.String("only", "", "only upgrade groupId:artifactId")
	repository := fs.String("repository", "", "Maven repository base URL")
	allowSnapshots := fs.Bool("allow-snapshots", false, "allow SNAPSHOT versions when selecting targets")
	allowMajor := fs.Bool("allow-major", true, "allow major version upgrades")
	allowMinor := fs.Bool("allow-minor", true, "allow minor version upgrades")
	allowIncremental := fs.Bool("allow-incremental", true, "allow patch/incremental version upgrades")
	allowDowngrade := fs.Bool("allow-downgrade", false, "allow downgrading versions")
	includePreReleases := fs.Bool("include-pre-releases", false, "include alpha/beta/rc versions")
	noBackup := fs.Bool("no-backup", false, "skip automatic backup before upgrade")
	backupDir := fs.String("backup-dir", "", "backup storage directory")
	quiet := fs.Bool("quiet", false, "disable progress output")
	verbose := fs.Bool("verbose", false, "show detailed progress output")
	summary := fs.Bool("summary", false, "print final summary report to stdout")
	includeRaw := fs.String("include", "", "only upgrade groupId:artifactId list")
	ignoreRaw := fs.String("ignore", "", "skip groupId:artifactId list")
	selectUpgrade := fs.Bool("select", false, "interactively choose packages to upgrade")
	ignoreSelect := fs.Bool("ignore-select", false, "interactively choose packages to ignore")
	pomPath := fs.String("pom", "", "use a specific pom.xml for list/select")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	flagOverrides, err := parseCoordinateFlags(*includeRaw, *ignoreRaw)
	if err != nil {
		return err
	}
	flagOverrides.Repository = *repository
	flagOverrides.BackupDir = *backupDir
	flagOverrides.Policy = *policyName

	if *selectUpgrade {
		selected, err := promptIncludedPackages(target, *recursive, *includeParent, *pomPath)
		if err != nil {
			return err
		}
		flagOverrides.Include = maven.MergeCoordinateLists(flagOverrides.Include, selected)
	}
	if *ignoreSelect {
		ignored, err := promptIgnoredPackages(target, *recursive, *includeParent, *pomPath)
		if err != nil {
			return err
		}
		flagOverrides.Ignore = maven.MergeCoordinateLists(flagOverrides.Ignore, ignored)
	}

	resolved, absTarget, err := resolveRuntimeConfig(target, flagOverrides)
	if err != nil {
		return err
	}

	policyValue := resolved.Policy
	if changedFlag(fs, "policy") {
		policyValue = *policyName
	}
	policy, err := maven.ParsePolicy(policyValue)
	if err != nil {
		return err
	}

	opts := maven.DefaultPolicyOptions(policy)
	if changedFlag(fs, "allow-snapshots") {
		opts.AllowSnapshots = *allowSnapshots
	}
	if changedFlag(fs, "allow-major") {
		opts.AllowMajorUpdates = *allowMajor
	}
	if changedFlag(fs, "allow-minor") {
		opts.AllowMinorUpdates = *allowMinor
	}
	if changedFlag(fs, "allow-incremental") {
		opts.AllowIncrementalUpdates = *allowIncremental
	}
	if changedFlag(fs, "allow-downgrade") {
		opts.AllowDowngrade = *allowDowngrade
	}
	if changedFlag(fs, "include-pre-releases") {
		opts.IgnorePreReleases = !*includePreReleases
	}

	filter, err := resolved.CoordinateFilter()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*only) != "" {
		filter, err = maven.NewCoordinateFilter([]string{*only}, nil)
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

	report, err := maven.Upgrade(maven.UpgradeRequest{
		Root:            target,
		Recursive:       *recursive,
		Policy:          policy,
		Options:         opts,
		Repository:      repositoryForProject(resolved, absTarget),
		DryRun:          *dryRun,
		IncludeParent:   *includeParent,
		OnlyCoordinate:  *only,
		AutoBackup:      autoBackup,
		BackupDir:       backupStore,
		BackupKeepCount: resolved.BackupKeepCount,
		Progress:        maven.DefaultProgress(*quiet, *verbose),
		Filter:          filter,
	})
	if err != nil {
		return err
	}

	if *summary || *quiet {
		fmt.Print(maven.FormatReport(report, *dryRun))
	}
	return nil
}
