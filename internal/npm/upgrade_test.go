package npm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubRegistry struct {
	versions map[string][]string
}

func (s *stubRegistry) ListVersions(name string) ([]string, error) {
	versions, ok := s.versions[name]
	if !ok {
		return nil, ErrPackageNotFound
	}
	return versions, nil
}

func TestUpgradePackageJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	content := map[string]any{
		"name": "demo",
		"dependencies": map[string]string{
			"left-pad": "^1.0.0",
		},
		"devDependencies": map[string]string{
			"eslint": "~7.32.0",
		},
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRegistry{versions: map[string][]string{
		"left-pad": {"1.0.0", "1.3.0"},
		"eslint":   {"7.32.0", "8.57.0"},
	}}

	report, err := Upgrade(UpgradeRequest{
		Root:        dir,
		Target:      TargetLatest,
		Registry:    repo,
		AutoBackup:  false,
		DepSections: dependencySections,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed != 2 {
		t.Fatalf("changed = %d, want 2", report.Changed)
	}

	pkg, err := ParsePackageFile(path)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := pkg.Dependencies(dependencySections)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, dep := range deps {
		byName[dep.Name] = dep.Version
	}
	if byName["left-pad"] != "^1.3.0" {
		t.Fatalf("left-pad = %q, want ^1.3.0", byName["left-pad"])
	}
	if byName["eslint"] != "~8.57.0" {
		t.Fatalf("eslint = %q, want ~8.57.0", byName["eslint"])
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "demo"`) {
		t.Fatalf("name field lost after upgrade:\n%s", raw)
	}
}

func TestUpgradeDryRunNoWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	original := []byte(`{
  "dependencies": {
    "left-pad": "^1.0.0"
  }
}
`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRegistry{versions: map[string][]string{
		"left-pad": {"1.0.0", "1.3.0"},
	}}

	report, err := Upgrade(UpgradeRequest{
		Root:       dir,
		Target:     TargetLatest,
		Registry:   repo,
		DryRun:     true,
		AutoBackup: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed != 1 {
		t.Fatalf("changed = %d, want 1", report.Changed)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("file modified during dry-run")
	}
}

func TestPackageFilterWildcard(t *testing.T) {
	filter, err := NewPackageFilter(nil, []string{"@types/*"})
	if err != nil {
		t.Fatal(err)
	}
	if allowed, _ := filter.Allows("@types/node"); allowed {
		t.Fatal("expected @types/node to be ignored")
	}
	if allowed, _ := filter.Allows("react"); !allowed {
		t.Fatal("expected react to be allowed")
	}
}
