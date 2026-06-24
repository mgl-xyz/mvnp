package maven

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRetention(t *testing.T) {
	root := t.TempDir()
	backupStore := filepath.Join(root, "backups")
	pomPath := filepath.Join(root, "pom.xml")

	for version := 1; version <= 3; version++ {
		if err := os.WriteFile(pomPath, []byte(fmt.Sprintf(`<project><artifactId>demo</artifactId><version>%d.0.0</version></project>`, version)), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := Backup(BackupRequest{
			Root:      root,
			BackupDir: backupStore,
			KeepCount: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if report.Manifest.Version != version {
			t.Fatalf("backup %d: got version v%d", version, report.Manifest.Version)
		}
	}

	manifests, err := ListBackups(backupStore)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 retained backups, got %d", len(manifests))
	}
	if manifests[0].Version != 2 || manifests[1].Version != 3 {
		t.Fatalf("expected versions [2 3], got [%d %d]", manifests[0].Version, manifests[1].Version)
	}

	restored, err := Restore(RestoreRequest{Root: root, BackupDir: backupStore, Version: 1})
	if err == nil {
		t.Fatalf("expected restore v1 to fail, got %+v", restored)
	}

	latest, err := Restore(RestoreRequest{Root: root, BackupDir: backupStore, Version: 0})
	if err != nil {
		t.Fatal(err)
	}
	if latest.Manifest.Version != 3 {
		t.Fatalf("expected latest v3, got v%d", latest.Manifest.Version)
	}

	data, err := os.ReadFile(pomPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `<project><artifactId>demo</artifactId><version>3.0.0</version></project>` {
		t.Fatalf("unexpected restored content: %q", string(data))
	}

	_, err = Restore(RestoreRequest{Root: root, BackupDir: backupStore, Version: 2})
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(pomPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `<project><artifactId>demo</artifactId><version>2.0.0</version></project>` {
		t.Fatalf("restore v2 failed, got %q", string(data))
	}
}

func TestBackupAndRestore(t *testing.T) {
	root := t.TempDir()
	backupStore := filepath.Join(root, "backups")

	mainPOM := filepath.Join(root, "pom.xml")
	moduleDir := filepath.Join(root, "module-a")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modulePOM := filepath.Join(moduleDir, "pom.xml")

	content := []byte(`<project><artifactId>demo</artifactId><version>1.0.0</version></project>`)
	if err := os.WriteFile(mainPOM, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modulePOM, []byte(`<project><artifactId>module-a</artifactId><version>1.0.0</version></project>`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Backup(BackupRequest{
		Root:      root,
		Recursive: true,
		BackupDir: backupStore,
		Label:     "initial",
		KeepCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.Version != 1 {
		t.Fatalf("expected backup v1, got v%d", first.Manifest.Version)
	}
	if len(first.Manifest.Files) != 2 {
		t.Fatalf("expected 2 backed up files, got %d", len(first.Manifest.Files))
	}

	if err := os.WriteFile(mainPOM, []byte(`<project><artifactId>demo</artifactId><version>2.0.0</version></project>`), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := Backup(BackupRequest{Root: root, BackupDir: backupStore, KeepCount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.Version != 2 {
		t.Fatalf("expected backup v2, got v%d", second.Manifest.Version)
	}

	manifests, err := ListBackups(backupStore)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(manifests))
	}

	latest, err := Restore(RestoreRequest{Root: root, BackupDir: backupStore, Version: 0})
	if err != nil {
		t.Fatal(err)
	}
	if latest.Manifest.Version != 2 {
		t.Fatalf("expected restore latest v2, got v%d", latest.Manifest.Version)
	}

	data, err := os.ReadFile(mainPOM)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `<project><artifactId>demo</artifactId><version>2.0.0</version></project>` {
		t.Fatalf("latest restore should keep v2 content, got %q", string(data))
	}

	firstRestore, err := Restore(RestoreRequest{Root: root, BackupDir: backupStore, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if firstRestore.Files != 2 {
		t.Fatalf("expected 2 restored files, got %d", firstRestore.Files)
	}

	data, err = os.ReadFile(mainPOM)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("restore v1 failed, got %q", string(data))
	}
}

func TestResolveBackupDirDefault(t *testing.T) {
	root := t.TempDir()
	dir, err := ResolveBackupDir(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, DefaultBackupDirName)
	if dir != want {
		t.Fatalf("ResolveBackupDir() = %q, want %q", dir, want)
	}
}
