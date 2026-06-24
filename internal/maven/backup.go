package maven

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultBackupDirName = ".mvnp/back"

// DefaultBackupKeepCount is the number of backup versions retained by default.
const DefaultBackupKeepCount = 2

// BackupManifest describes one versioned pom.xml backup snapshot.
type BackupManifest struct {
	Version   int          `json:"version"`
	CreatedAt time.Time    `json:"createdAt"`
	Root      string       `json:"root"`
	Label     string       `json:"label,omitempty"`
	Files     []BackupFile `json:"files"`
}

// BackupFile maps a backed-up pom.xml to its original location.
type BackupFile struct {
	RelativePath string `json:"relativePath"`
	SourcePath   string `json:"sourcePath"`
}

// BackupRequest configures a backup operation.
type BackupRequest struct {
	Root      string
	Recursive bool
	BackupDir string
	Label     string
	KeepCount int
}

// BackupReport summarizes a completed backup.
type BackupReport struct {
	Manifest BackupManifest
	StoreDir string
}

// RestoreRequest configures a restore operation.
type RestoreRequest struct {
	Root      string
	BackupDir string
	Version   int
}

// RestoreReport summarizes a completed restore.
type RestoreReport struct {
	Manifest BackupManifest
	Files    int
}

// ResolveBackupDir returns the directory used to store backups.
func ResolveBackupDir(projectRoot, customDir string) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(customDir) == "" {
		return filepath.Join(root, DefaultBackupDirName), nil
	}
	backupDir, err := filepath.Abs(customDir)
	if err != nil {
		return "", err
	}
	return backupDir, nil
}

// Backup creates a new versioned snapshot of pom.xml files.
func Backup(request BackupRequest) (*BackupReport, error) {
	root, err := filepath.Abs(request.Root)
	if err != nil {
		return nil, err
	}

	pomFiles, err := FindPOMFiles(request.Root, request.Recursive)
	if err != nil {
		return nil, err
	}

	backupDir, err := ResolveBackupDir(root, request.BackupDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, err
	}

	nextVersion, err := nextBackupVersion(backupDir)
	if err != nil {
		return nil, err
	}

	versionDir := versionDirectory(backupDir, nextVersion)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return nil, err
	}

	manifest := BackupManifest{
		Version:   nextVersion,
		CreatedAt: time.Now().UTC(),
		Root:      root,
		Label:     strings.TrimSpace(request.Label),
	}

	for _, pomPath := range pomFiles {
		absPOM, err := filepath.Abs(pomPath)
		if err != nil {
			return nil, err
		}

		rel, err := filepath.Rel(root, absPOM)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)

		destPath := filepath.Join(versionDir, rel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, err
		}
		if err := copyFile(absPOM, destPath); err != nil {
			return nil, fmt.Errorf("backup %s: %w", absPOM, err)
		}

		manifest.Files = append(manifest.Files, BackupFile{
			RelativePath: rel,
			SourcePath:   absPOM,
		})
	}

	if err := writeManifest(versionDir, manifest); err != nil {
		return nil, err
	}

	if err := pruneOldBackups(backupDir, request.KeepCount); err != nil {
		return nil, err
	}

	return &BackupReport{
		Manifest: manifest,
		StoreDir: versionDir,
	}, nil
}

// ListBackups returns all backup manifests sorted by version ascending.
func ListBackups(backupDir string) ([]BackupManifest, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifests []BackupManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version, ok := parseVersionDirName(entry.Name())
		if !ok {
			continue
		}
		manifest, err := readManifest(filepath.Join(backupDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if manifest.Version == 0 {
			manifest.Version = version
		}
		manifests = append(manifests, manifest)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Version < manifests[j].Version
	})
	return manifests, nil
}

// Restore restores pom.xml files from a backup version.
// Version <= 0 restores the latest backup.
func Restore(request RestoreRequest) (*RestoreReport, error) {
	root, err := filepath.Abs(request.Root)
	if err != nil {
		return nil, err
	}

	backupDir, err := ResolveBackupDir(root, request.BackupDir)
	if err != nil {
		return nil, err
	}

	manifest, versionDir, err := resolveBackupVersion(backupDir, request.Version)
	if err != nil {
		return nil, err
	}

	restored := 0
	for _, file := range manifest.Files {
		src := filepath.Join(versionDir, filepath.FromSlash(file.RelativePath))
		dest := filepath.Join(root, filepath.FromSlash(file.RelativePath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := copyFile(src, dest); err != nil {
			return nil, fmt.Errorf("restore %s: %w", file.RelativePath, err)
		}
		restored++
	}

	return &RestoreReport{
		Manifest: manifest,
		Files:    restored,
	}, nil
}

func resolveBackupVersion(backupDir string, version int) (BackupManifest, string, error) {
	manifests, err := ListBackups(backupDir)
	if err != nil {
		return BackupManifest{}, "", err
	}
	if len(manifests) == 0 {
		return BackupManifest{}, "", fmt.Errorf("no backups found in %s", backupDir)
	}

	var selected BackupManifest
	if version <= 0 {
		selected = manifests[len(manifests)-1]
	} else {
		found := false
		for _, item := range manifests {
			if item.Version == version {
				selected = item
				found = true
				break
			}
		}
		if !found {
			return BackupManifest{}, "", fmt.Errorf("backup version %d not found in %s", version, backupDir)
		}
	}

	versionDir := versionDirectory(backupDir, selected.Version)
	if _, err := os.Stat(versionDir); err != nil {
		return BackupManifest{}, "", fmt.Errorf("backup version directory missing: %s", versionDir)
	}
	return selected, versionDir, nil
}

func nextBackupVersion(backupDir string) (int, error) {
	manifests, err := ListBackups(backupDir)
	if err != nil {
		return 0, err
	}
	if len(manifests) == 0 {
		return 1, nil
	}
	return manifests[len(manifests)-1].Version + 1, nil
}

// pruneOldBackups removes oldest backup versions, keeping only the latest keepCount entries.
// keepCount <= 0 retains all backups.
func pruneOldBackups(backupDir string, keepCount int) error {
	if keepCount <= 0 {
		return nil
	}

	manifests, err := ListBackups(backupDir)
	if err != nil {
		return err
	}
	if len(manifests) <= keepCount {
		return nil
	}

	for _, item := range manifests[:len(manifests)-keepCount] {
		dir := versionDirectory(backupDir, item.Version)
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove backup v%d: %w", item.Version, err)
		}
	}
	return nil
}

func versionDirectory(backupDir string, version int) string {
	return filepath.Join(backupDir, fmt.Sprintf("v%05d", version))
}

func parseVersionDirName(name string) (int, bool) {
	if !strings.HasPrefix(name, "v") {
		return 0, false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(name, "v"))
	if err != nil || version <= 0 {
		return 0, false
	}
	return version, true
}

func manifestPath(versionDir string) string {
	return filepath.Join(versionDir, "manifest.json")
}

func writeManifest(versionDir string, manifest BackupManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(manifestPath(versionDir), data, 0o644)
}

func readManifest(versionDir string) (BackupManifest, error) {
	data, err := os.ReadFile(manifestPath(versionDir))
	if err != nil {
		return BackupManifest{}, err
	}
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func FormatBackupReport(report *BackupReport) string {
	if report == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("backup v%d created at %s\n", report.Manifest.Version, report.Manifest.CreatedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("store: %s\n", report.StoreDir))
	for _, file := range report.Manifest.Files {
		b.WriteString(fmt.Sprintf("  %s\n", file.RelativePath))
	}
	b.WriteString(fmt.Sprintf("%d pom.xml file(s) backed up\n", len(report.Manifest.Files)))
	return b.String()
}

func FormatRestoreReport(report *RestoreReport) string {
	if report == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("restored backup v%d from %s\n", report.Manifest.Version, report.Manifest.CreatedAt.Format(time.RFC3339)))
	for _, file := range report.Manifest.Files {
		b.WriteString(fmt.Sprintf("  %s\n", file.RelativePath))
	}
	b.WriteString(fmt.Sprintf("%d pom.xml file(s) restored\n", report.Files))
	return b.String()
}
