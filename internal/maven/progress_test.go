package maven

import (
	"bytes"
	"strings"
	"testing"
)

func TestDockerStyleProgressOutput(t *testing.T) {
	var buf bytes.Buffer
	progress := NewDockerStyleProgress(&buf)
	progress.Start(2, 2)
	progress.Prewarm(1, 2, "com.example:demo")
	progress.Checked(UpgradeResult{
		GroupID:     "com.example",
		ArtifactID:  "demo",
		ResolvedOld: "1.0.0",
		NewVersion:  "2.0.0",
	})
	progress.Checked(UpgradeResult{
		GroupID:     "com.example",
		ArtifactID:  "other",
		Skipped:     true,
		Reason:      "already up to date",
		ResolvedOld: "2.0.0",
	})
	progress.Finish(&UpgradeReport{Results: []UpgradeResult{
		{NewVersion: "2.0.0"},
		{Skipped: true, Reason: "already up to date"},
	}, Changed: 1}, true)

	out := buf.String()
	for _, want := range []string{
		"Checking 2 packages",
		"com.example:demo: Resolving",
		"com.example:demo: 1.0.0 -> 2.0.0",
		"2 entries, 1 would upgrade, 1 up to date",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("progress output missing %q:\n%s", want, out)
		}
	}
}

func TestShortSkipReasonRateLimit(t *testing.T) {
	reason := ShortSkipReason("metadata request failed (429): This tool has been identified as abusive")
	if reason != "Rate limited" {
		t.Fatalf("got %q", reason)
	}
}

func TestShortSkipReasonProperty(t *testing.T) {
	reason := ShortSkipReason("property not found in <properties>: central-publishing-maven-plugin.version")
	if reason != "Property missing: central-publishing-maven-plugin.version" {
		t.Fatalf("got %q", reason)
	}
}

func TestRenderProgressBar(t *testing.T) {
	bar := renderProgressBar(5, 10, 10)
	if !strings.Contains(bar, "50%") || !strings.Contains(bar, "5/10") {
		t.Fatalf("unexpected progress bar: %q", bar)
	}
}
