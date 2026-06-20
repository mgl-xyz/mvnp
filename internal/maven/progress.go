package maven

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Progress reports upgrade activity to the user.
type Progress interface {
	Start(entries, uniquePackages int)
	Prewarm(index, total int, coord string)
	Checking(index, total int, coord, pomPath string, cached bool)
	Checked(result UpgradeResult)
	BackingUp()
	BackupDone(report *BackupReport)
	Writing(files int)
	Finish(report *UpgradeReport, dryRun bool)
}

// NopProgress discards progress events.
type NopProgress struct{}

func (NopProgress) Start(int, int)                            {}
func (NopProgress) Prewarm(int, int, string)                  {}
func (NopProgress) Checking(int, int, string, string, bool) {}
func (NopProgress) Checked(UpgradeResult)                   {}
func (NopProgress) BackingUp()                              {}
func (NopProgress) BackupDone(*BackupReport)                {}
func (NopProgress) Writing(int)                             {}
func (NopProgress) Finish(*UpgradeReport, bool)             {}

// DockerStyleProgress shows compact docker pull-like status lines.
type DockerStyleProgress struct {
	Out             io.Writer
	terminal        bool
	rateLimitWarned bool
}

func NewDockerStyleProgress(out io.Writer) *DockerStyleProgress {
	if out == nil {
		out = os.Stderr
	}
	return &DockerStyleProgress{
		Out:      out,
		terminal: isTerminal(out),
	}
}

func (p *DockerStyleProgress) Start(entries, uniquePackages int) {
	if uniquePackages > 0 && uniquePackages < entries {
		fmt.Fprintf(p.Out, "Checking %d entries (%d packages)\n\n", entries, uniquePackages)
		return
	}
	fmt.Fprintf(p.Out, "Checking %d packages\n\n", entries)
}

func (p *DockerStyleProgress) Prewarm(index, total int, coord string) {
	line := fmt.Sprintf("%s: Resolving %s", shortCoord(coord), dockerProgressBar(index, total))
	p.writeActive(line)
}

func (p *DockerStyleProgress) Checking(int, int, string, string, bool) {}

func (p *DockerStyleProgress) Checked(result UpgradeResult) {
	coord := shortCoord(ArtifactCoordinate(result.GroupID, result.ArtifactID))
	if result.Skipped {
		p.printSkip(coord, result.Reason)
		return
	}
	old := formatUpgradeVersion(result)
	p.writeFinal(fmt.Sprintf("%s: %s -> %s", coord, old, result.NewVersion))
}

func (p *DockerStyleProgress) printSkip(coord, reason string) {
	short := ShortSkipReason(reason)
	switch short {
	case "Rate limited":
		if !p.rateLimitWarned {
			p.writeFinal("Warning: Maven Central rate limit (429). Use -repository <mirror> or retry later.")
			p.rateLimitWarned = true
		}
	case "Up to date":
		if reason != "already up to date" {
			p.writeFinal(fmt.Sprintf("%s: %s", coord, short))
		}
	default:
		if reason == "already up to date" {
			return
		}
		p.writeFinal(fmt.Sprintf("%s: %s", coord, short))
	}
}

func (p *DockerStyleProgress) BackingUp() {
	p.writeFinal("Backup: creating snapshot")
}

func (p *DockerStyleProgress) BackupDone(report *BackupReport) {
	if report == nil {
		return
	}
	p.writeFinal(fmt.Sprintf("Backup: v%d saved", report.Manifest.Version))
}

func (p *DockerStyleProgress) Writing(files int) {
	p.writeFinal(fmt.Sprintf("Writing: %d pom.xml", files))
}

func (p *DockerStyleProgress) Finish(report *UpgradeReport, dryRun bool) {
	if report == nil {
		return
	}
	upgraded, skipped, upToDate := summarizeResults(report.Results)
	skipBreakdown := summarizeSkipReasons(report.Results)
	action := "upgraded"
	if dryRun {
		action = "would upgrade"
	}

	fmt.Fprintf(p.Out, "\n%d entries, %d %s, %d up to date", len(report.Results), upgraded, action, upToDate)
	if skipped > 0 {
		fmt.Fprintf(p.Out, ", %d skipped", skipped)
		if skipBreakdown != "" {
			fmt.Fprintf(p.Out, " (%s)", skipBreakdown)
		}
	}
	fmt.Fprintln(p.Out)
}

func (p *DockerStyleProgress) writeActive(line string) {
	if p.terminal {
		fmt.Fprintf(p.Out, "\r\033[2K%s", line)
		return
	}
	fmt.Fprintln(p.Out, line)
}

func (p *DockerStyleProgress) writeFinal(line string) {
	if p.terminal {
		fmt.Fprintf(p.Out, "\r\033[2K%s\n", line)
		return
	}
	fmt.Fprintln(p.Out, line)
}

// VerboseProgress prints detailed multi-line progress output.
type VerboseProgress struct {
	Out io.Writer
}

func NewVerboseProgress(out io.Writer) *VerboseProgress {
	if out == nil {
		out = os.Stderr
	}
	return &VerboseProgress{Out: out}
}

func (p *VerboseProgress) Start(entries, uniquePackages int) {
	fmt.Fprintf(p.Out, "scanning %d entries (%d unique packages)...\n\n", entries, uniquePackages)
}

func (p *VerboseProgress) Prewarm(index, total int, coord string) {
	fmt.Fprintf(p.Out, "  [%d/%d] resolve %s\n", index, total, coord)
}

func (p *VerboseProgress) Checking(index, total int, coord, pomPath string, cached bool) {
	bar := renderProgressBar(index, total, 28)
	fmt.Fprintf(p.Out, "%s %s\n", bar, shortPath(pomPath))
	if cached {
		fmt.Fprintf(p.Out, "  -> checking %s (cached)\n", coord)
		return
	}
	fmt.Fprintf(p.Out, "  -> querying %s\n", coord)
}

func (p *VerboseProgress) Checked(result UpgradeResult) {
	fmt.Fprintf(p.Out, "  %s\n\n", formatVerboseResult(result))
}

func (p *VerboseProgress) BackingUp() {
	fmt.Fprintln(p.Out, "creating backup before upgrade...")
}

func (p *VerboseProgress) BackupDone(report *BackupReport) {
	if report == nil {
		return
	}
	fmt.Fprintf(p.Out, "backup v%d saved (%d file(s))\n\n", report.Manifest.Version, len(report.Manifest.Files))
}

func (p *VerboseProgress) Writing(files int) {
	fmt.Fprintf(p.Out, "writing %d pom.xml file(s)...\n\n", files)
}

func (p *VerboseProgress) Finish(report *UpgradeReport, dryRun bool) {
	if report == nil {
		return
	}
	action := "updated"
	if dryRun {
		action = "would update"
	}
	fmt.Fprintf(p.Out, "done: %d dependencies %s\n", report.Changed, action)
}

func dockerProgressBar(current, total int) string {
	if total <= 0 {
		total = 1
	}
	const width = 20
	filled := current * width / total
	if filled > width {
		filled = width
	}
	pct := current * 100 / total
	return fmt.Sprintf("[%s%s] %3d%% %d/%d",
		strings.Repeat("=", filled),
		strings.Repeat(" ", width-filled),
		pct,
		current,
		total,
	)
}

func dockerResultStatus(item UpgradeResult) string {
	if item.Skipped {
		if item.Reason == "already up to date" {
			return "Up to date"
		}
		return ShortSkipReason(item.Reason)
	}
	old := formatUpgradeVersion(item)
	return fmt.Sprintf("%s -> %s", old, item.NewVersion)
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

func shortCoord(coord string) string {
	const max = 52
	if len(coord) <= max {
		return coord
	}
	parts := strings.SplitN(coord, ":", 2)
	if len(parts) == 2 && len(parts[1]) > 24 {
		return parts[0] + ":" + parts[1][:21] + "..."
	}
	return coord[:max-3] + "..."
}

func formatVerboseResult(item UpgradeResult) string {
	coord := ArtifactCoordinate(item.GroupID, item.ArtifactID)
	version := formatUpgradeVersion(item)
	if item.Skipped {
		return fmt.Sprintf("skip  %s (%s) %s - %s", coord, item.Section, version, ShortSkipReason(item.Reason))
	}
	return fmt.Sprintf("ok    %s (%s) %s -> %s", coord, item.Section, version, item.NewVersion)
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

func renderProgressBar(current, total, width int) string {
	if total <= 0 {
		total = 1
	}
	if current > total {
		current = total
	}
	if width < 10 {
		width = 10
	}
	filled := current * width / total
	if filled > width {
		filled = width
	}
	empty := width - filled
	pct := current * 100 / total
	return fmt.Sprintf("[%s%s] %3d%% (%d/%d)",
		strings.Repeat("=", filled),
		strings.Repeat(" ", empty),
		pct,
		current,
		total,
	)
}

func shortPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if len(path) <= 48 {
		return path
	}
	return "..." + path[len(path)-45:]
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func DefaultProgress(quiet, verbose bool) Progress {
	if quiet {
		return NopProgress{}
	}
	if verbose {
		return NewVerboseProgress(os.Stderr)
	}
	return NewDockerStyleProgress(os.Stderr)
}
