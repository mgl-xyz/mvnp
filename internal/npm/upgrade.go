package npm

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type UpgradeRequest struct {
	Root            string
	Recursive       bool
	Target          Target
	Options         TargetOptions
	Registry        VersionLister
	DryRun          bool
	OnlyPackage     string
	AutoBackup      bool
	BackupDir       string
	BackupKeepCount int
	BackupLabel     string
	Progress        Progress
	Filter          PackageFilter
	DepSections     []string
}

type UpgradeResult struct {
	PackagePath string
	Name        string
	OldVersion  string
	NewVersion  string
	Section     string
	ResolvedOld string
	Skipped     bool
	Reason      string
}

type UpgradeReport struct {
	Results      []UpgradeResult
	Changed      int
	BackupReport *BackupReport
}

type pendingChange struct {
	section string
	name    string
	version string
}

type packagePending struct {
	pkg     *PackageFile
	changes []pendingChange
}

type upgradeWorkItem struct {
	packagePath string
	pkg         *PackageFile
	dep         Dependency
}

func Upgrade(request UpgradeRequest) (*UpgradeReport, error) {
	target := request.Target
	if request.Options == (TargetOptions{}) {
		request.Options = DefaultTargetOptions(target)
	}
	if request.Registry == nil {
		request.Registry = NewCachingRegistry("", "")
	}
	progress := request.Progress
	if progress == nil {
		progress = NopProgress{}
	}

	packageFiles, err := FindPackageFiles(request.Root, request.Recursive)
	if err != nil {
		return nil, err
	}

	workItems, err := collectUpgradeWorkItems(packageFiles, request.DepSections, request.OnlyPackage)
	if err != nil {
		return nil, err
	}
	workItems = filterWorkItems(workItems, request.Filter)

	if len(workItems) == 0 {
		progress.Start(0, 0)
		report := &UpgradeReport{}
		progress.Finish(report, request.DryRun)
		return report, nil
	}

	uniqueNames := uniquePackageNames(workItems)
	progress.Start(len(workItems), len(uniqueNames))

	report := &UpgradeReport{}
	versionCache := make(map[string][]string)
	prewarmPackages(uniqueNames, request.Registry, versionCache, progress)
	pendingByFile := make(map[string]*packagePending)

	for _, item := range workItems {
		result, change := evaluateDependencyUpgrade(
			item.dep,
			target,
			request.Options,
			versionCache,
			request.Registry,
		)
		result.PackagePath = item.packagePath
		report.Results = append(report.Results, result)
		progress.Checked(result)

		if result.Skipped {
			continue
		}

		report.Changed++
		pending := pendingByFile[item.packagePath]
		if pending == nil {
			pending = &packagePending{pkg: item.pkg}
			pendingByFile[item.packagePath] = pending
		}
		if change != nil {
			pending.changes = append(pending.changes, *change)
		}
	}

	var pendingFiles []packagePending
	for _, pending := range pendingByFile {
		if len(pending.changes) > 0 {
			pendingFiles = append(pendingFiles, *pending)
		}
	}

	if report.Changed == 0 || request.DryRun {
		progress.Finish(report, request.DryRun)
		return report, nil
	}

	if request.AutoBackup {
		progress.BackingUp()
		label := request.BackupLabel
		if label == "" {
			label = "pre-upgrade"
		}
		backupReport, err := Backup(BackupRequest{
			Root:      request.Root,
			Recursive: request.Recursive,
			BackupDir: request.BackupDir,
			Label:     label,
			KeepCount: request.BackupKeepCount,
		})
		if err != nil {
			return nil, fmt.Errorf("auto backup before upgrade: %w", err)
		}
		report.BackupReport = backupReport
		progress.BackupDone(backupReport)
	}

	progress.Writing(len(pendingFiles))
	for _, item := range pendingFiles {
		for _, change := range item.changes {
			if err := item.pkg.SetDependency(change.section, change.name, change.version); err != nil {
				return nil, err
			}
		}
		if err := item.pkg.Save(); err != nil {
			return nil, fmt.Errorf("write %s: %w", item.pkg.Path, err)
		}
	}

	progress.Finish(report, request.DryRun)
	return report, nil
}

func collectUpgradeWorkItems(packageFiles, sections []string, only string) ([]upgradeWorkItem, error) {
	var items []upgradeWorkItem
	for _, path := range packageFiles {
		pkg, err := ParsePackageFile(path)
		if err != nil {
			return nil, err
		}
		deps, err := pkg.UpgradeableDependencies(sections, only)
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			items = append(items, upgradeWorkItem{
				packagePath: path,
				pkg:         pkg,
				dep:         dep,
			})
		}
	}
	return items, nil
}

func uniquePackageNames(items []upgradeWorkItem) []string {
	seen := make(map[string]bool)
	var names []string
	for _, item := range items {
		if seen[item.dep.Name] {
			continue
		}
		seen[item.dep.Name] = true
		names = append(names, item.dep.Name)
	}
	sort.Strings(names)
	return names
}

func prewarmPackages(names []string, registry VersionLister, versionCache map[string][]string, progress Progress) {
	for index, name := range names {
		progress.Prewarm(index+1, len(names), name)
		versions, err := registry.ListVersions(name)
		if err == nil {
			versionCache[name] = versions
		}
	}
}

func evaluateDependencyUpgrade(
	dep Dependency,
	target Target,
	opts TargetOptions,
	versionCache map[string][]string,
	registry VersionLister,
) (UpgradeResult, *pendingChange) {
	result := UpgradeResult{
		Name:       dep.Name,
		OldVersion: dep.Version,
		Section:    dep.Section,
	}

	resolved, ok := ResolveSpecVersion(dep.Version)
	if !ok {
		result.Skipped = true
		result.Reason = "unsupported version expression"
		return result, nil
	}
	result.ResolvedOld = resolved

	chosen, reason, ok := lookupTargetVersion(dep.Name, resolved, target, opts, versionCache, registry)
	if !ok {
		result.Skipped = true
		result.Reason = reason
		return result, nil
	}

	newSpec := UpgradeSpec(dep.Version, chosen)
	if newSpec == dep.Version {
		result.Skipped = true
		result.Reason = "already up to date"
		return result, nil
	}

	result.NewVersion = newSpec
	return result, &pendingChange{
		section: dep.Section,
		name:    dep.Name,
		version: newSpec,
	}
}

func lookupTargetVersion(
	name, current string,
	target Target,
	opts TargetOptions,
	versionCache map[string][]string,
	registry VersionLister,
) (string, string, bool) {
	versions, ok := versionCache[name]
	if !ok {
		var err error
		versions, err = registry.ListVersions(name)
		if err != nil {
			return "", skipReasonFromError(err), false
		}
		versionCache[name] = versions
	}
	chosen, ok := SelectTargetVersion(current, versions, target, opts)
	if !ok {
		return "", "no matching version for target", false
	}
	return chosen, "", true
}

func filterWorkItems(items []upgradeWorkItem, filter PackageFilter) []upgradeWorkItem {
	if !filter.HasInclude() && len(filter.ignore) == 0 {
		return items
	}
	var filtered []upgradeWorkItem
	for _, item := range items {
		allowed, _ := filter.Allows(item.dep.Name)
		if allowed {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func FormatReport(report *UpgradeReport, dryRun bool) string {
	if report == nil {
		return "no dependencies found to evaluate"
	}

	var b strings.Builder
	if report.BackupReport != nil {
		b.WriteString(FormatBackupReport(report.BackupReport))
		b.WriteString("\n")
	}

	if len(report.Results) == 0 {
		b.WriteString("no dependencies found to evaluate")
		return b.String()
	}

	action := "updated"
	if dryRun {
		action = "would update"
	}

	for _, item := range report.Results {
		if item.Skipped {
			b.WriteString(fmt.Sprintf("skip  %s (%s) %s - %s\n", item.Name, item.Section, formatUpgradeVersion(item), ShortSkipReason(item.Reason)))
			continue
		}
		b.WriteString(fmt.Sprintf("%s %s (%s) %s -> %s\n", action, item.Name, item.Section, formatUpgradeVersion(item), item.NewVersion))
	}

	b.WriteString(fmt.Sprintf("\n%d dependencies %s\n", report.Changed, action))
	return b.String()
}

func formatUpgradeVersion(item UpgradeResult) string {
	if item.ResolvedOld != "" && item.ResolvedOld != item.OldVersion {
		return fmt.Sprintf("%s [%s]", item.OldVersion, item.ResolvedOld)
	}
	if item.ResolvedOld != "" {
		return item.OldVersion
	}
	return item.OldVersion
}

func SummarizeReport(report *UpgradeReport, dryRun bool) string {
	upgraded, skipped, upToDate := summarizeResults(report.Results)
	skipBreakdown := summarizeSkipReasons(report.Results)
	action := "upgraded"
	if dryRun {
		action = "would upgrade"
	}
	line := fmt.Sprintf("%d entries, %d %s, %d up to date", len(report.Results), upgraded, action, upToDate)
	if skipped > 0 {
		line += fmt.Sprintf(", %d skipped", skipped)
		if skipBreakdown != "" {
			line += fmt.Sprintf(" (%s)", skipBreakdown)
		}
	}
	return line
}

func summarizeResults(results []UpgradeResult) (upgraded, skipped, upToDate int) {
	for _, item := range results {
		if item.Skipped {
			if item.Reason == "already up to date" {
				upToDate++
			} else {
				skipped++
			}
			continue
		}
		upgraded++
	}
	return upgraded, skipped, upToDate
}

func summarizeSkipReasons(results []UpgradeResult) string {
	counts := make(map[string]int)
	for _, item := range results {
		if !item.Skipped || item.Reason == "already up to date" {
			continue
		}
		short := ShortSkipReason(item.Reason)
		counts[short]++
	}
	if len(counts) == 0 {
		return ""
	}
	var parts []string
	for reason, count := range counts {
		parts = append(parts, fmt.Sprintf("%s: %d", reason, count))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func skipReasonFromError(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return ErrRateLimited.Error()
	}
	if errors.Is(err, ErrPackageNotFound) {
		return ErrPackageNotFound.Error()
	}
	return err.Error()
}

func ShortSkipReason(reason string) string {
	switch {
	case reason == "already up to date":
		return "Up to date"
	case reason == ErrRateLimited.Error():
		return "Rate limited"
	case strings.Contains(reason, ErrPackageNotFound.Error()):
		return "Not found"
	case reason == "ignored":
		return "Ignored"
	case reason == "not selected":
		return "Not selected"
	case reason == "unsupported version expression":
		return "Unsupported spec"
	case reason == "no matching version for target":
		return "No match"
	default:
		if len(reason) > 48 {
			return reason[:45] + "..."
		}
		return reason
	}
}
