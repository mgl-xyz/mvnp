package maven

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GlobalSettingsDir  = ".config/mvnp"
	GlobalSettingsName = "settings.json"
	ProjectSettingsDir = ".mvnp"
	ProjectSettingsName = "settings.json"
)

// Settings stores shared mvnp configuration.
type Settings struct {
	Repository       string                       `json:"repository,omitempty"`
	BackupDir        string                       `json:"backupDir,omitempty"`
	MetadataCacheDir string                       `json:"metadataCacheDir,omitempty"`
	Policy           string                       `json:"policy,omitempty"`
	Ignore           []string                     `json:"ignore,omitempty"`
	Include          []string                     `json:"include,omitempty"`
	Projects         map[string]ProjectSettings   `json:"projects,omitempty"`
}

// ProjectSettings overrides settings for a specific project path.
type ProjectSettings struct {
	Repository       string   `json:"repository,omitempty"`
	BackupDir        string   `json:"backupDir,omitempty"`
	MetadataCacheDir string   `json:"metadataCacheDir,omitempty"`
	Policy           string   `json:"policy,omitempty"`
	Ignore           []string `json:"ignore,omitempty"`
	Include          []string `json:"include,omitempty"`
}

// ResolvedSettings merges global and project settings with CLI overrides.
type ResolvedSettings struct {
	Repository       string
	BackupDir        string
	MetadataCacheDir string
	Policy           string
	Include          []string
	Ignore           []string
	GlobalPath       string
	ProjectPath      string
}

type SettingsOverrides struct {
	Repository       string
	BackupDir        string
	MetadataCacheDir string
	Policy           string
	Include          []string
	Ignore           []string
}

func DefaultSettings() Settings {
	return Settings{
		Repository:       defaultRepository,
		BackupDir:        DefaultBackupDirName,
		MetadataCacheDir: metadataCacheDir,
		Policy:           string(PolicyLatestReleases),
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
	resolved := ResolvedSettings{
		Repository:       defaults.Repository,
		BackupDir:        defaults.BackupDir,
		MetadataCacheDir: defaults.MetadataCacheDir,
		Policy:           defaults.Policy,
	}

	globalPath, err := GlobalSettingsPath()
	if err == nil {
		if globalSettings, err := LoadSettingsFile(globalPath); err != nil {
			return ResolvedSettings{}, err
		} else if _, statErr := os.Stat(globalPath); statErr == nil {
			resolved.GlobalPath = globalPath
			applySettings(&resolved, globalSettings, false)
		}
	}

	projectPath, err := ProjectSettingsPath(projectRoot)
	if err == nil {
		if projectSettings, err := LoadSettingsFile(projectPath); err != nil {
			return ResolvedSettings{}, err
		} else if _, statErr := os.Stat(projectPath); statErr == nil {
			resolved.ProjectPath = projectPath
			applySettings(&resolved, projectSettings, true)
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

func applySettings(resolved *ResolvedSettings, settings Settings, project bool) {
	if settings.Repository != "" {
		resolved.Repository = settings.Repository
	}
	if settings.BackupDir != "" {
		resolved.BackupDir = settings.BackupDir
	}
	if settings.MetadataCacheDir != "" {
		resolved.MetadataCacheDir = settings.MetadataCacheDir
	}
	if settings.Policy != "" {
		resolved.Policy = settings.Policy
	}
	if len(settings.Ignore) > 0 {
		resolved.Ignore = MergeCoordinateLists(resolved.Ignore, settings.Ignore)
	}
	if len(settings.Include) > 0 {
		resolved.Include = MergeCoordinateLists(resolved.Include, settings.Include)
	}
	_ = project
}

func applyProjectSettings(resolved *ResolvedSettings, settings ProjectSettings) {
	applySettings(resolved, Settings{
		Repository:       settings.Repository,
		BackupDir:        settings.BackupDir,
		MetadataCacheDir: settings.MetadataCacheDir,
		Policy:           settings.Policy,
		Ignore:           settings.Ignore,
		Include:          settings.Include,
	}, true)
}

func applyOverrides(resolved *ResolvedSettings, overrides SettingsOverrides) {
	if strings.TrimSpace(overrides.Repository) != "" {
		resolved.Repository = strings.TrimSpace(overrides.Repository)
	}
	if strings.TrimSpace(overrides.BackupDir) != "" {
		resolved.BackupDir = strings.TrimSpace(overrides.BackupDir)
	}
	if strings.TrimSpace(overrides.MetadataCacheDir) != "" {
		resolved.MetadataCacheDir = strings.TrimSpace(overrides.MetadataCacheDir)
	}
	if strings.TrimSpace(overrides.Policy) != "" {
		resolved.Policy = strings.TrimSpace(overrides.Policy)
	}
	if len(overrides.Ignore) > 0 {
		resolved.Ignore = MergeCoordinateLists(resolved.Ignore, overrides.Ignore)
	}
	if len(overrides.Include) > 0 {
		resolved.Include = MergeCoordinateLists(resolved.Include, overrides.Include)
	}
}

func (r ResolvedSettings) CoordinateFilter() (CoordinateFilter, error) {
	return NewCoordinateFilter(r.Include, r.Ignore)
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
	settings.Ignore = []string{
		"com.fasterxml.jackson:jackson-bom",
		"tools.jackson:jackson-bom",
		"software.amazon.awssdk:bom",
	}
	if err := SaveSettingsFile(path, settings); err != nil {
		return "", err
	}
	return path, nil
}
