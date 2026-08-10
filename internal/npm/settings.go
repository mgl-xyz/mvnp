package npm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GlobalSettingsDir   = ".config/nvnp"
	GlobalSettingsName  = "settings.json"
	ProjectSettingsDir  = ".nvnp"
	ProjectSettingsName = "settings.json"
)

type Settings struct {
	Registry         string                     `json:"registry,omitempty"`
	BackupDir        string                     `json:"backupDir,omitempty"`
	AutoBackup       *bool                      `json:"autoBackup,omitempty"`
	BackupKeepCount  *int                       `json:"backupKeepCount,omitempty"`
	MetadataCacheDir string                     `json:"metadataCacheDir,omitempty"`
	Target           string                     `json:"target,omitempty"`
	DepSections      []string                   `json:"depSections,omitempty"`
	Ignore           []string                   `json:"ignore,omitempty"`
	Include          []string                   `json:"include,omitempty"`
	Projects         map[string]ProjectSettings `json:"projects,omitempty"`
}

type ProjectSettings struct {
	Registry         string   `json:"registry,omitempty"`
	BackupDir        string   `json:"backupDir,omitempty"`
	AutoBackup       *bool    `json:"autoBackup,omitempty"`
	BackupKeepCount  *int     `json:"backupKeepCount,omitempty"`
	MetadataCacheDir string   `json:"metadataCacheDir,omitempty"`
	Target           string   `json:"target,omitempty"`
	DepSections      []string `json:"depSections,omitempty"`
	Ignore           []string `json:"ignore,omitempty"`
	Include          []string `json:"include,omitempty"`
}

type ResolvedSettings struct {
	Registry         string
	BackupDir        string
	AutoBackup       bool
	BackupKeepCount  int
	MetadataCacheDir string
	Target           string
	DepSections      []string
	Include          []string
	Ignore           []string
	GlobalPath       string
	ProjectPath      string
}

type SettingsOverrides struct {
	Registry         string
	BackupDir        string
	MetadataCacheDir string
	Target           string
	Include          []string
	Ignore           []string
}

func DefaultSettings() Settings {
	autoBackup := true
	keepCount := DefaultBackupKeepCount
	return Settings{
		Registry:         defaultRegistry,
		BackupDir:        DefaultBackupDirName,
		AutoBackup:       &autoBackup,
		BackupKeepCount:  &keepCount,
		MetadataCacheDir: metadataCacheDir,
		Target:           string(TargetLatest),
		DepSections:      append([]string(nil), dependencySections...),
	}
}

func GlobalSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, GlobalSettingsDir, GlobalSettingsName), nil
}

func ProjectSettingsPath(projectRoot string) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ProjectSettingsDir, ProjectSettingsName), nil
}

func LoadSettingsFile(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func SaveSettingsFile(path string, settings Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ResolveSettings(projectRoot string, overrides SettingsOverrides) (ResolvedSettings, error) {
	defaults := DefaultSettings()
	autoBackup := true
	if defaults.AutoBackup != nil {
		autoBackup = *defaults.AutoBackup
	}
	backupKeepCount := DefaultBackupKeepCount
	if defaults.BackupKeepCount != nil {
		backupKeepCount = *defaults.BackupKeepCount
	}
	resolved := ResolvedSettings{
		Registry:         defaults.Registry,
		BackupDir:        defaults.BackupDir,
		AutoBackup:       autoBackup,
		BackupKeepCount:  backupKeepCount,
		MetadataCacheDir: defaults.MetadataCacheDir,
		Target:           defaults.Target,
		DepSections:      append([]string(nil), defaults.DepSections...),
	}

	globalPath, err := GlobalSettingsPath()
	if err == nil {
		if globalSettings, err := LoadSettingsFile(globalPath); err != nil {
			return ResolvedSettings{}, err
		} else if _, statErr := os.Stat(globalPath); statErr == nil {
			resolved.GlobalPath = globalPath
			applySettings(&resolved, globalSettings)
		}
	}

	projectPath, err := ProjectSettingsPath(projectRoot)
	if err == nil {
		if projectSettings, err := LoadSettingsFile(projectPath); err != nil {
			return ResolvedSettings{}, err
		} else if _, statErr := os.Stat(projectPath); statErr == nil {
			resolved.ProjectPath = projectPath
			applySettings(&resolved, projectSettings)
		}
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return ResolvedSettings{}, err
	}
	if resolved.GlobalPath != "" {
		if globalSettings, err := LoadSettingsFile(resolved.GlobalPath); err == nil {
			if project, ok := globalSettings.Projects[absRoot]; ok {
				applyProjectSettings(&resolved, project)
			}
		}
	}

	applyOverrides(&resolved, overrides)
	return resolved, nil
}

func applySettings(resolved *ResolvedSettings, settings Settings) {
	if settings.Registry != "" {
		resolved.Registry = settings.Registry
	}
	if settings.BackupDir != "" {
		resolved.BackupDir = settings.BackupDir
	}
	if settings.AutoBackup != nil {
		resolved.AutoBackup = *settings.AutoBackup
	}
	if settings.BackupKeepCount != nil {
		resolved.BackupKeepCount = *settings.BackupKeepCount
	}
	if settings.MetadataCacheDir != "" {
		resolved.MetadataCacheDir = settings.MetadataCacheDir
	}
	if settings.Target != "" {
		resolved.Target = settings.Target
	}
	if len(settings.DepSections) > 0 {
		resolved.DepSections = append([]string(nil), settings.DepSections...)
	}
	if len(settings.Ignore) > 0 {
		resolved.Ignore = MergePackageLists(resolved.Ignore, settings.Ignore)
	}
	if len(settings.Include) > 0 {
		resolved.Include = MergePackageLists(resolved.Include, settings.Include)
	}
}

func applyProjectSettings(resolved *ResolvedSettings, settings ProjectSettings) {
	applySettings(resolved, Settings{
		Registry:         settings.Registry,
		BackupDir:        settings.BackupDir,
		AutoBackup:       settings.AutoBackup,
		BackupKeepCount:  settings.BackupKeepCount,
		MetadataCacheDir: settings.MetadataCacheDir,
		Target:           settings.Target,
		DepSections:      settings.DepSections,
		Ignore:           settings.Ignore,
		Include:          settings.Include,
	})
}

func applyOverrides(resolved *ResolvedSettings, overrides SettingsOverrides) {
	if strings.TrimSpace(overrides.Registry) != "" {
		resolved.Registry = strings.TrimSpace(overrides.Registry)
	}
	if strings.TrimSpace(overrides.BackupDir) != "" {
		resolved.BackupDir = strings.TrimSpace(overrides.BackupDir)
	}
	if strings.TrimSpace(overrides.MetadataCacheDir) != "" {
		resolved.MetadataCacheDir = strings.TrimSpace(overrides.MetadataCacheDir)
	}
	if strings.TrimSpace(overrides.Target) != "" {
		resolved.Target = strings.TrimSpace(overrides.Target)
	}
	if len(overrides.Ignore) > 0 {
		resolved.Ignore = MergePackageLists(resolved.Ignore, overrides.Ignore)
	}
	if len(overrides.Include) > 0 {
		resolved.Include = MergePackageLists(resolved.Include, overrides.Include)
	}
}

func (r ResolvedSettings) PackageFilter() (PackageFilter, error) {
	return NewPackageFilter(r.Include, r.Ignore)
}

func (r ResolvedSettings) AbsPath(projectRoot, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty path")
	}
	if filepath.IsAbs(value) {
		return value, nil
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, value), nil
}

func InitProjectSettings(projectRoot string) (string, error) {
	path, err := ProjectSettingsPath(projectRoot)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	settings := DefaultSettings()
	settings.Ignore = []string{"@types/node"}
	if err := SaveSettingsFile(path, settings); err != nil {
		return "", err
	}
	return path, nil
}
