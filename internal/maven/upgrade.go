package maven

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type UpgradeRequest struct {
	Root            string
	Recursive       bool
	Policy          Policy
	Options         PolicyOptions
	Repository      VersionLister
	DryRun          bool
	IncludeParent   bool
	OnlyCoordinate  string
	AutoBackup      bool
	BackupDir       string
	BackupKeepCount int
	BackupLabel     string
	Progress        Progress
	Filter          CoordinateFilter
}

type UpgradeResult struct {
	POMPath     string
	GroupID     string
	ArtifactID  string
	OldVersion  string
	NewVersion  string
	Section     string
	PropertyRef string
	ResolvedOld string
	Skipped     bool
	Reason      string
}

type UpgradeReport struct {
	Results      []UpgradeResult
	Changed      int
	BackupReport *BackupReport
}

type propertyDecision struct {
	writable   PropertyEntry
	resolved   string
	target     string
	ok         bool
	skipReason string
}

type pendingChange struct {
	kind     string
	dep      Dependency
	property PropertyEntry
	version  string
	pos      int
}

type pomPending struct {
	pom     *POM
	changes []pendingChange
}

type upgradeWorkItem struct {
	pomPath string
	pom     *POM
	dep     Dependency
}

func Upgrade(request UpgradeRequest) (*UpgradeReport, error) {
	policy := request.Policy
	if request.Options == (PolicyOptions{}) {
		request.Options = DefaultPolicyOptions(policy)
	}
	if request.Repository == nil {
		request.Repository = NewCachingRepository("", "")
	}
	progress := request.Progress
	if progress == nil {
		progress = NopProgress{}
	}

	pomFiles, err := FindPOMFiles(request.Root, request.Recursive)
	if err != nil {
		return nil, err
	}

	workItems, err := collectUpgradeWorkItems(pomFiles, request.IncludeParent, request.OnlyCoordinate)
	if err != nil {
		return nil, err
	}
	workItems = FilterWorkItems(workItems, request.Filter)

	if len(workItems) == 0 {
		progress.Start(0, 0)
		report := &UpgradeReport{}
		progress.Finish(report, request.DryRun)
		return report, nil
	}

	uniqueCoords := uniqueCoordinates(workItems)
	progress.Start(len(workItems), len(uniqueCoords))

	report := &UpgradeReport{}
	versionCache := make(map[string][]string)
	prewarmPackages(uniqueCoords, request.Repository, versionCache, progress)
	pendingByPOM := make(map[string]*pomPending)

	for _, item := range workItems {
		result, change := evaluateDependencyUpgrade(
			item.pom,
			item.dep,
			policy,
			request.Options,
			versionCache,
			request.Repository,
		)
		result.POMPath = item.pomPath
		report.Results = append(report.Results, result)
		progress.Checked(result)

		if result.Skipped {
			continue
		}

		report.Changed++
		pending := pendingByPOM[item.pomPath]
		if pending == nil {
			pending = &pomPending{pom: item.pom}
			pendingByPOM[item.pomPath] = pending
		}
		if change != nil {
			pending.changes = appendPendingChange(pending.changes, *change)
		}
	}

	var pendingPOMs []pomPending
	for _, pending := range pendingByPOM {
		if len(pending.changes) > 0 {
			pendingPOMs = append(pendingPOMs, *pending)
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

	progress.Writing(len(pendingPOMs))
	for _, item := range pendingPOMs {
		sort.Slice(item.changes, func(i, j int) bool {
			return item.changes[i].pos > item.changes[j].pos
		})
		for _, change := range item.changes {
			switch change.kind {
			case "property":
				item.pom.ApplyPropertyVersion(change.property, change.version)
			default:
				item.pom.ApplyVersion(change.dep, change.version)
			}
		}
		if err := item.pom.Save(); err != nil {
			return nil, fmt.Errorf("write %s: %w", item.pom.Path, err)
		}
	}

	progress.Finish(report, request.DryRun)
	return report, nil
}

func collectUpgradeWorkItems(pomFiles []string, includeParent bool, only string) ([]upgradeWorkItem, error) {
	var items []upgradeWorkItem
	for _, pomPath := range pomFiles {
		pom, err := ParsePOM(pomPath)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", pomPath, err)
		}
		for _, dep := range pom.UpgradeableDependencies(includeParent, only) {
			items = append(items, upgradeWorkItem{
				pomPath: pomPath,
				pom:     pom,
				dep:     dep,
			})
		}
	}
	return items, nil
}

func uniqueCoordinates(items []upgradeWorkItem) []string {
	seen := make(map[string]bool)
	var coords []string
	for _, item := range items {
		coord := ArtifactCoordinate(item.dep.GroupID, item.dep.ArtifactID)
		if seen[coord] {
			continue
		}
		seen[coord] = true
		coords = append(coords, coord)
	}
	return coords
}

func prewarmPackages(coords []string, repository VersionLister, versionCache map[string][]string, progress Progress) {
	for index, coord := range coords {
		progress.Prewarm(index+1, len(coords), coord)
		groupID, artifactID, err := ParseArtifactCoordinate(coord)
		if err != nil {
			continue
		}
		versions, err := repository.ListVersions(groupID, artifactID)
		if err == nil {
			versionCache[coord] = versions
		}
	}
}

func evaluateDependencyUpgrade(
	pom *POM,
	dep Dependency,
	policy Policy,
	opts PolicyOptions,
	versionCache map[string][]string,
	repository VersionLister,
) (UpgradeResult, *pendingChange) {
	result := UpgradeResult{
		GroupID:    dep.GroupID,
		ArtifactID: dep.ArtifactID,
		OldVersion: dep.Version,
		Section:    dep.Section,
	}

	if !dep.HasExplicitVersion() {
		result.Skipped = true
		result.Reason = "no explicit version in pom"
		return result, nil
	}

	propertyRef, viaProperty := ExtractPropertyRef(dep.Version)
	if viaProperty {
		result.PropertyRef = dep.Version
		decision := evaluatePropertyUpgrade(pom, propertyRef, dep, policy, opts, versionCache, repository)
		result.ResolvedOld = decision.resolved
		if !decision.ok {
			result.Skipped = true
			result.Reason = decision.skipReason
			return result, nil
		}
		result.NewVersion = decision.target
		return result, &pendingChange{
			kind:     "property",
			property: decision.writable,
			version:  decision.target,
			pos:      decision.writable.ValuePos,
		}
	}

	current := pom.ResolveVersion(dep.Version)
	if ClassifyVersion(current) == VersionUnknown {
		result.Skipped = true
		result.Reason = "unsupported version expression"
		return result, nil
	}

	target, reason, ok := lookupTargetVersion(dep.GroupID, dep.ArtifactID, current, policy, opts, versionCache, repository)
	if !ok {
		result.Skipped = true
		result.Reason = reason
		return result, nil
	}
	if target == current {
		result.Skipped = true
		result.Reason = "already up to date"
		return result, nil
	}

	result.NewVersion = target
	result.ResolvedOld = current
	return result, &pendingChange{
		kind:    "direct",
		dep:     dep,
		version: target,
		pos:     dep.VersionPos,
	}
}

func appendPendingChange(changes []pendingChange, change pendingChange) []pendingChange {
	if change.kind == "property" {
		for _, existing := range changes {
			if existing.kind == "property" && existing.property.Key == change.property.Key {
				return changes
			}
		}
	}
	return append(changes, change)
}

func evaluatePropertyUpgrade(
	pom *POM,
	propertyRef string,
	dep Dependency,
	policy Policy,
	opts PolicyOptions,
	versionCache map[string][]string,
	repository VersionLister,
) propertyDecision {
	writable, resolved, ok := pom.WritableProperty(propertyRef)
	if !ok {
		return propertyDecision{skipReason: "property not found in <properties>: " + propertyRef}
	}
	if ClassifyVersion(resolved) == VersionUnknown {
		return propertyDecision{skipReason: "property " + propertyRef + " has unsupported version expression"}
	}

	target, reason, ok := lookupTargetVersion(dep.GroupID, dep.ArtifactID, resolved, policy, opts, versionCache, repository)
	if !ok {
		return propertyDecision{
			writable:   writable,
			resolved:   resolved,
			skipReason: reason,
		}
	}
	if target == resolved {
		return propertyDecision{
			writable:   writable,
			resolved:   resolved,
			skipReason: "already up to date",
		}
	}

	return propertyDecision{
		writable: writable,
		resolved: resolved,
		target:   target,
		ok:       true,
	}
}

func lookupTargetVersion(
	groupID, artifactID, current string,
	policy Policy,
	opts PolicyOptions,
	versionCache map[string][]string,
	repository VersionLister,
) (string, string, bool) {
	cacheKey := ArtifactCoordinate(groupID, artifactID)
	versions, ok := versionCache[cacheKey]
	if !ok {
		var err error
		versions, err = repository.ListVersions(groupID, artifactID)
		if err != nil {
			return "", skipReasonFromError(err), false
		}
		versionCache[cacheKey] = versions
	}
	target, ok := SelectVersion(current, versions, policy, opts)
	if !ok {
		return "", "no matching version for policy", false
	}
	return target, "", true
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
		coord := ArtifactCoordinate(item.GroupID, item.ArtifactID)
		if item.Skipped {
			b.WriteString(fmt.Sprintf("skip  %s (%s) %s -> %s\n", coord, item.Section, formatUpgradeVersion(item), ShortSkipReason(item.Reason)))
			continue
		}
		b.WriteString(fmt.Sprintf("%s %s (%s) %s -> %s\n", action, coord, item.Section, formatUpgradeVersion(item), item.NewVersion))
	}

	b.WriteString(fmt.Sprintf("\n%d dependencies %s\n", report.Changed, action))
	return b.String()
}

func formatUpgradeVersion(item UpgradeResult) string {
	if item.PropertyRef != "" {
		if item.ResolvedOld != "" {
			return fmt.Sprintf("%s [%s]", item.PropertyRef, item.ResolvedOld)
		}
		return item.PropertyRef
	}
	if item.ResolvedOld != "" {
		return item.ResolvedOld
	}
	return item.OldVersion
}

func skipReasonFromError(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return ErrRateLimited.Error()
	}
	return err.Error()
}
