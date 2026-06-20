package maven

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoordinateFilter(t *testing.T) {
	filter, err := NewCoordinateFilter(
		[]string{"com.example:demo"},
		[]string{"com.example:skip"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := filter.Allows("com.example", "demo"); !ok {
		t.Fatal("expected demo allowed")
	}
	if ok, reason := filter.Allows("com.example", "skip"); ok || reason != "ignored" {
		t.Fatalf("expected ignored, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := filter.Allows("other", "lib"); ok || reason != "not selected" {
		t.Fatalf("expected not selected, got ok=%v reason=%q", ok, reason)
	}
}

func TestResolveSettingsMerge(t *testing.T) {
	project := t.TempDir()
	projectSettings := filepath.Join(project, ProjectSettingsDir, ProjectSettingsName)
	if err := os.MkdirAll(filepath.Dir(projectSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectSettings, []byte(`{
  "repository": "https://repo.example.com/maven2",
  "ignore": ["com.fasterxml.jackson:jackson-bom"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveSettings(project, SettingsOverrides{
		Ignore: []string{"org.junit:junit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Repository != "https://repo.example.com/maven2" {
		t.Fatalf("repository = %q", resolved.Repository)
	}
	if len(resolved.Ignore) != 2 {
		t.Fatalf("ignore = %#v", resolved.Ignore)
	}
}

func TestParseSelection(t *testing.T) {
	indices, err := parseSelection("1,3-5", 6)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 3, 4, 5}
	if len(indices) != len(want) {
		t.Fatalf("got %#v", indices)
	}
	for i := range want {
		if indices[i] != want[i] {
			t.Fatalf("got %#v want %#v", indices, want)
		}
	}
}
