package npm

import (
	"fmt"
	"io"
	"os"
)

type Progress interface {
	Start(entries, uniquePackages int)
	Prewarm(index, total int, name string)
	Checked(result UpgradeResult)
	BackingUp()
	BackupDone(report *BackupReport)
	Writing(files int)
	Finish(report *UpgradeReport, dryRun bool)
}

type NopProgress struct{}

func (NopProgress) Start(int, int)              {}
func (NopProgress) Prewarm(int, int, string)    {}
func (NopProgress) Checked(UpgradeResult)       {}
func (NopProgress) BackingUp()                  {}
func (NopProgress) BackupDone(*BackupReport)    {}
func (NopProgress) Writing(int)                 {}
func (NopProgress) Finish(*UpgradeReport, bool) {}

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

func (p *DockerStyleProgress) Prewarm(index, total int, name string) {
	p.writeActive(formatProgressLine(name, index, total))
}

func (p *DockerStyleProgress) Checked(result UpgradeResult) {
	if result.Skipped {
		p.printSkip(result)
		return
	}
	p.writeFinal(formatResultLine(result))
}

func (p *DockerStyleProgress) printSkip(result UpgradeResult) {
	short := ShortSkipReason(result.Reason)
	switch short {
	case "Rate limited":
		if !p.rateLimitWarned {
			p.writeFinal("Warning: npm registry rate limit (429). Use -registry <mirror> or retry later.")
			p.rateLimitWarned = true
		}
	case "Up to date":
		if result.Reason != "already up to date" {
			p.writeFinal(formatSkipLine(result.Name, short))
		}
	default:
		if result.Reason == "already up to date" {
			return
		}
		p.writeFinal(formatSkipLine(result.Name, short))
	}
}

func (p *DockerStyleProgress) BackingUp() {
	p.writeFinal("Backup: creating snapshot")
}

func (p *DockerStyleProgress) BackupDone(report *BackupReport) {
	if report == nil {
		return
	}
	p.writeFinal(formatBackupDone(report.Manifest.Version))
}

func (p *DockerStyleProgress) Writing(files int) {
	p.writeFinal(formatWriting(files))
}

func (p *DockerStyleProgress) Finish(report *UpgradeReport, dryRun bool) {
	if report == nil {
		return
	}
	fmt.Fprintln(p.Out)
	fmt.Fprintln(p.Out, SummarizeReport(report, dryRun))
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

func (p *VerboseProgress) Prewarm(index, total int, name string) {
	fmt.Fprintf(p.Out, "  [%d/%d] resolve %s\n", index, total, name)
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
	fmt.Fprintf(p.Out, "writing %d package.json file(s)...\n\n", files)
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

func DefaultProgress(quiet, verbose bool) Progress {
	if quiet {
		return NopProgress{}
	}
	if verbose {
		return NewVerboseProgress(os.Stderr)
	}
	return NewDockerStyleProgress(os.Stderr)
}

func formatProgressLine(name string, index, total int) string {
	if total <= 0 {
		total = 1
	}
	const width = 20
	filled := index * width / total
	if filled > width {
		filled = width
	}
	pct := index * 100 / total
	return fmt.Sprintf("%s: Resolving [%s%s] %3d%% %d/%d",
		shortName(name),
		repeat("=", filled),
		repeat(" ", width-filled),
		pct,
		index,
		total,
	)
}

func formatResultLine(result UpgradeResult) string {
	return fmt.Sprintf("%s: %s -> %s", shortName(result.Name), formatUpgradeVersion(result), result.NewVersion)
}

func formatSkipLine(name, reason string) string {
	return fmt.Sprintf("%s: %s", shortName(name), reason)
}

func formatBackupDone(version int) string {
	return fmt.Sprintf("Backup: v%d saved", version)
}

func formatWriting(files int) string {
	return fmt.Sprintf("Writing: %d package.json", files)
}

func formatVerboseResult(result UpgradeResult) string {
	version := formatUpgradeVersion(result)
	if result.Skipped {
		return fmt.Sprintf("skip  %s (%s) %s - %s", result.Name, result.Section, version, ShortSkipReason(result.Reason))
	}
	return fmt.Sprintf("ok    %s (%s) %s -> %s", result.Name, result.Section, version, result.NewVersion)
}

func shortName(name string) string {
	const max = 52
	if len(name) <= max {
		return name
	}
	return name[:max-3] + "..."
}

func repeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*count)
	for i := 0; i < count; i++ {
		out = append(out, s...)
	}
	return string(out)
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
