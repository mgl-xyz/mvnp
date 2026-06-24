package maven

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Dependency struct {
	GroupID    string
	ArtifactID string
	Version    string
	Scope      string
	Section    string
	Start      int
	End        int
	VersionPos int
	VersionLen int
}

// PropertyEntry tracks a <properties> entry and its value position in the pom content.
type PropertyEntry struct {
	Key      string
	Value    string
	ValuePos int
	ValueLen int
}

type POM struct {
	Path         string
	Content      string
	Properties   map[string]string
	PropertyList []PropertyEntry
	Parent       *Dependency
	Deps         []Dependency
}

var (
	propertyRefPattern = regexp.MustCompile(`\$\{([^}]+)\}`)
	depBlockPattern    = regexp.MustCompile(`(?is)<dependency>(.*?)</dependency>`)
	pluginBlockPattern = regexp.MustCompile(`(?is)<plugin>(.*?)</plugin>`)
	pathBlockPattern   = regexp.MustCompile(`(?is)<path>(.*?)</path>`)
	pluginDepsPattern  = regexp.MustCompile(`(?is)<dependencies>(.*?)</dependencies>`)
	tagPattern         = func(tag string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`(?is)<%s>\s*([^<]*?)\s*</%s>`, tag, tag))
	}
)

func FindPOMFiles(root string, recursive bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(root), "pom.xml") {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a pom.xml file", root)
	}

	pomPath := filepath.Join(root, "pom.xml")
	if _, err := os.Stat(pomPath); err != nil {
		return nil, fmt.Errorf("pom.xml not found in %s", root)
	}

	if !recursive {
		return []string{pomPath}, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "target" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), "pom.xml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no pom.xml files found under %s", root)
	}
	return files, nil
}

func ParsePOM(path string) (*POM, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(contentBytes)

	pom := &POM{
		Path:    path,
		Content: content,
	}
	pom.PropertyList, pom.Properties = parseProperties(content)

	if parent := parseSingleDependency(content, "parent"); parent != nil {
		pom.Parent = parent
	}

	for _, section := range []string{"dependencies", "dependencyManagement"} {
		pom.Deps = append(pom.Deps, parseSectionDependencies(content, section)...)
	}
	for _, section := range []string{"plugins", "pluginManagement"} {
		pom.Deps = append(pom.Deps, parseSectionPlugins(content, section)...)
	}

	return pom, nil
}

func parseProperties(content string) ([]PropertyEntry, map[string]string) {
	props := make(map[string]string)
	sectionPattern := regexp.MustCompile(`(?is)<properties>(.*?)</properties>`)
	match := sectionPattern.FindStringSubmatchIndex(content)
	if match == nil {
		return nil, props
	}

	sectionContent := content[match[2]:match[3]]
	sectionOffset := match[2]
	keyPattern := regexp.MustCompile(`(?is)<([a-zA-Z0-9_.-]+)>\s*([^<]*?)\s*</[a-zA-Z0-9_.-]+>`)

	var entries []PropertyEntry
	for _, loc := range keyPattern.FindAllStringSubmatchIndex(sectionContent, -1) {
		key := strings.TrimSpace(sectionContent[loc[2]:loc[3]])
		value := strings.TrimSpace(sectionContent[loc[4]:loc[5]])
		entry := PropertyEntry{
			Key:      key,
			Value:    value,
			ValuePos: sectionOffset + loc[4],
			ValueLen: loc[5] - loc[4],
		}
		entries = append(entries, entry)
		props[key] = value
	}
	return entries, props
}

func parseSectionDependencies(content, section string) []Dependency {
	if section == "dependencies" {
		return parseTopLevelSectionDependencies(content)
	}
	return parseWrappedSectionDependencies(content, section)
}

func parseTopLevelSectionDependencies(content string) []Dependency {
	excludeSpans := sectionSpans(content, "dependencyManagement")
	return parseDependenciesFromSectionBlocks(content, "dependencies", excludeSpans)
}

func parseWrappedSectionDependencies(content, section string) []Dependency {
	sectionPattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>(.*?)</%s>`, section, section))
	sectionMatch := sectionPattern.FindStringSubmatchIndex(content)
	if sectionMatch == nil {
		return nil
	}
	sectionContent := content[sectionMatch[2]:sectionMatch[3]]
	sectionOffset := sectionMatch[2]
	return parseDependenciesFromContent(sectionContent, section, sectionOffset)
}

func parseDependenciesFromSectionBlocks(content, section string, excludeSpans [][2]int) []Dependency {
	sectionPattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>(.*?)</%s>`, section, section))
	var deps []Dependency
	for _, loc := range sectionPattern.FindAllStringSubmatchIndex(content, -1) {
		if spanInsideExcluded(loc[0], loc[1], excludeSpans) {
			continue
		}
		sectionContent := content[loc[2]:loc[3]]
		deps = append(deps, parseDependenciesFromContent(sectionContent, section, loc[2])...)
	}
	return deps
}

func parseDependenciesFromContent(sectionContent, section string, sectionOffset int) []Dependency {
	var deps []Dependency
	for _, loc := range depBlockPattern.FindAllStringSubmatchIndex(sectionContent, -1) {
		block := sectionContent[loc[2]:loc[3]]
		dep := parseDependencyBlock(block, section)
		if dep == nil {
			continue
		}
		dep.Start = sectionOffset + loc[0]
		dep.End = sectionOffset + loc[1]
		versionTag := tagPattern("version")
		versionLoc := versionTag.FindStringSubmatchIndex(block)
		if versionLoc != nil {
			dep.VersionPos = sectionOffset + loc[2] + versionLoc[2]
			dep.VersionLen = versionLoc[3] - versionLoc[2]
		}
		deps = append(deps, *dep)
	}
	return deps
}

func parseSingleDependency(content, section string) *Dependency {
	sectionPattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>(.*?)</%s>`, section, section))
	sectionMatch := sectionPattern.FindStringSubmatchIndex(content)
	if sectionMatch == nil {
		return nil
	}
	block := content[sectionMatch[2]:sectionMatch[3]]
	dep := parseDependencyBlock(block, section)
	if dep == nil {
		return nil
	}
	dep.Start = sectionMatch[0]
	dep.End = sectionMatch[1]
	if versionLoc := tagPattern("version").FindStringSubmatchIndex(block); versionLoc != nil {
		dep.VersionPos = sectionMatch[2] + versionLoc[2]
		dep.VersionLen = versionLoc[3] - versionLoc[2]
	}
	return dep
}

func parseSectionPlugins(content, section string) []Dependency {
	if section == "plugins" {
		return parseTopLevelSectionPlugins(content)
	}
	return parseWrappedSectionPlugins(content, section)
}

func parseTopLevelSectionPlugins(content string) []Dependency {
	excludeSpans := sectionSpans(content, "pluginManagement")
	return parsePluginsFromSectionBlocks(content, "plugins", excludeSpans)
}

func parseWrappedSectionPlugins(content, section string) []Dependency {
	sectionPattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>(.*?)</%s>`, section, section))
	sectionMatch := sectionPattern.FindStringSubmatchIndex(content)
	if sectionMatch == nil {
		return nil
	}
	sectionContent := content[sectionMatch[2]:sectionMatch[3]]
	sectionOffset := sectionMatch[2]
	return parsePluginsFromContent(sectionContent, section, sectionOffset)
}

func parsePluginsFromSectionBlocks(content, section string, excludeSpans [][2]int) []Dependency {
	sectionPattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>(.*?)</%s>`, section, section))
	var deps []Dependency
	for _, loc := range sectionPattern.FindAllStringSubmatchIndex(content, -1) {
		if spanInsideExcluded(loc[0], loc[1], excludeSpans) {
			continue
		}
		sectionContent := content[loc[2]:loc[3]]
		deps = append(deps, parsePluginsFromContent(sectionContent, section, loc[2])...)
	}
	return deps
}

func parsePluginsFromContent(sectionContent, section string, sectionOffset int) []Dependency {
	var deps []Dependency
	for _, loc := range pluginBlockPattern.FindAllStringSubmatchIndex(sectionContent, -1) {
		block := sectionContent[loc[2]:loc[3]]
		blockStart := sectionOffset + loc[2]
		deps = append(deps, parsePluginBlock(block, section, blockStart)...)
	}
	return deps
}

func parsePluginBlock(block, section string, blockStart int) []Dependency {
	var deps []Dependency

	header := pluginHeader(block)
	if dep := parseDependencyBlock(header, section); dep != nil {
		dep.Start = blockStart
		dep.End = blockStart + len(block)
		if versionLoc := tagPattern("version").FindStringSubmatchIndex(header); versionLoc != nil {
			dep.VersionPos = blockStart + versionLoc[2]
			dep.VersionLen = versionLoc[3] - versionLoc[2]
		}
		deps = append(deps, *dep)
	}

	pathSection := section + ":path"
	for _, loc := range pathBlockPattern.FindAllStringSubmatchIndex(block, -1) {
		pathContent := block[loc[2]:loc[3]]
		dep := parseDependencyBlock(pathContent, pathSection)
		if dep == nil {
			continue
		}
		pathStart := blockStart + loc[0]
		dep.Start = pathStart
		dep.End = blockStart + loc[1]
		if versionLoc := tagPattern("version").FindStringSubmatchIndex(pathContent); versionLoc != nil {
			dep.VersionPos = blockStart + loc[2] + versionLoc[2]
			dep.VersionLen = versionLoc[3] - versionLoc[2]
		}
		deps = append(deps, *dep)
	}

	pluginDepSection := section + ":dependency"
	if depsLoc := pluginDepsPattern.FindStringSubmatchIndex(block); depsLoc != nil {
		depsContent := block[depsLoc[2]:depsLoc[3]]
		depsOffset := blockStart + depsLoc[2]
		for _, loc := range depBlockPattern.FindAllStringSubmatchIndex(depsContent, -1) {
			depContent := depsContent[loc[2]:loc[3]]
			dep := parseDependencyBlock(depContent, pluginDepSection)
			if dep == nil {
				continue
			}
			depStart := depsOffset + loc[0]
			dep.Start = depStart
			dep.End = depsOffset + loc[1]
			if versionLoc := tagPattern("version").FindStringSubmatchIndex(depContent); versionLoc != nil {
				dep.VersionPos = depsOffset + loc[2] + versionLoc[2]
				dep.VersionLen = versionLoc[3] - versionLoc[2]
			}
			deps = append(deps, *dep)
		}
	}

	return deps
}

func pluginHeader(block string) string {
	lower := strings.ToLower(block)
	for _, tag := range []string{"<configuration", "<dependencies", "<executions"} {
		if idx := strings.Index(lower, tag); idx >= 0 {
			return block[:idx]
		}
	}
	return block
}

func parseDependencyBlock(block, section string) *Dependency {
	groupID, ok := extractTag(block, "groupId")
	if !ok || strings.TrimSpace(groupID) == "" {
		return nil
	}
	artifactID, ok := extractTag(block, "artifactId")
	if !ok || strings.TrimSpace(artifactID) == "" {
		return nil
	}
	version, ok := extractTag(block, "version")
	if !ok || strings.TrimSpace(version) == "" {
		return nil
	}
	scope, _ := extractTag(block, "scope")

	return &Dependency{
		GroupID:    strings.TrimSpace(groupID),
		ArtifactID: strings.TrimSpace(artifactID),
		Version:    strings.TrimSpace(version),
		Scope:      strings.TrimSpace(scope),
		Section:    section,
	}
}

// HasExplicitVersion reports whether the dependency has its own <version> tag in the pom.
func (dep Dependency) HasExplicitVersion() bool {
	return dep.VersionPos > 0 && strings.TrimSpace(dep.Version) != ""
}

func extractTag(block, tag string) (string, bool) {
	pattern := tagPattern(tag)
	match := pattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return "", false
	}
	return match[1], true
}

func stripSection(content, section string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>.*?</%s>`, section, section))
	return pattern.ReplaceAllString(content, "")
}

func sectionSpans(content, section string) [][2]int {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?is)<%s>.*?</%s>`, section, section))
	var spans [][2]int
	for _, loc := range pattern.FindAllStringIndex(content, -1) {
		spans = append(spans, [2]int{loc[0], loc[1]})
	}
	return spans
}

func spanInsideExcluded(start, end int, excludeSpans [][2]int) bool {
	for _, span := range excludeSpans {
		if start >= span[0] && end <= span[1] {
			return true
		}
	}
	return false
}

func (p *POM) ResolveVersion(version string) string {
	seen := make(map[string]bool)
	return p.resolveVersion(version, seen)
}

func ExtractPropertyRef(version string) (string, bool) {
	version = strings.TrimSpace(version)
	match := propertyRefPattern.FindStringSubmatch(version)
	if len(match) < 2 || match[0] != version {
		return "", false
	}
	return match[1], true
}

func (p *POM) PropertyByKey(key string) (PropertyEntry, bool) {
	for _, entry := range p.PropertyList {
		if entry.Key == key {
			return entry, true
		}
	}
	return PropertyEntry{}, false
}

// WritableProperty follows property reference chains and returns the entry whose value should be updated.
func (p *POM) WritableProperty(refKey string) (PropertyEntry, string, bool) {
	seen := make(map[string]bool)
	key := refKey
	for {
		if seen[key] {
			return PropertyEntry{}, "", false
		}
		seen[key] = true

		entry, ok := p.PropertyByKey(key)
		if !ok {
			return PropertyEntry{}, "", false
		}
		if nextKey, ok := ExtractPropertyRef(entry.Value); ok {
			key = nextKey
			continue
		}
		return entry, strings.TrimSpace(entry.Value), true
	}
}

func (p *POM) resolveVersion(version string, seen map[string]bool) string {
	version = strings.TrimSpace(version)
	match := propertyRefPattern.FindStringSubmatch(version)
	if len(match) < 2 {
		return version
	}
	key := match[1]
	if seen[key] {
		return version
	}
	seen[key] = true
	if value, ok := p.Properties[key]; ok {
		return p.resolveVersion(value, seen)
	}
	return version
}

func (p *POM) ApplyVersion(dep Dependency, newVersion string) {
	if dep.VersionPos <= 0 || dep.VersionLen <= 0 {
		return
	}
	p.Content = p.Content[:dep.VersionPos] + newVersion + p.Content[dep.VersionPos+dep.VersionLen:]
}

func (p *POM) ApplyPropertyVersion(entry PropertyEntry, newVersion string) {
	if entry.ValuePos <= 0 || entry.ValueLen <= 0 {
		return
	}
	p.Content = p.Content[:entry.ValuePos] + newVersion + p.Content[entry.ValuePos+entry.ValueLen:]
	p.Properties[entry.Key] = newVersion
}

func (p *POM) Save() error {
	return os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

func (p *POM) UpgradeableDependencies(includeParent bool, only string) []Dependency {
	var deps []Dependency
	if includeParent && p.Parent != nil && p.Parent.HasExplicitVersion() {
		deps = append(deps, *p.Parent)
	}
	for _, dep := range p.Deps {
		if !dep.HasExplicitVersion() {
			continue
		}
		deps = append(deps, dep)
	}

	if only == "" {
		return deps
	}

	groupID, artifactID, err := ParseArtifactCoordinate(only)
	if err != nil {
		return nil
	}
	var filtered []Dependency
	for _, dep := range deps {
		if dep.GroupID == groupID && dep.ArtifactID == artifactID {
			filtered = append(filtered, dep)
		}
	}
	return filtered
}
