package npm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const packageFileName = "package.json"

// Default upgrade scope: prod + dev only.
var dependencySections = []string{
	"dependencies",
	"devDependencies",
}

// Dependency describes one package.json dependency entry.
type Dependency struct {
	Name    string
	Version string
	Section string
}

type depLocation struct {
	Section    string
	Name       string
	Version    string
	ValueStart int
	ValueEnd   int
}

// PackageFile represents a parsed package.json with byte-accurate editing.
type PackageFile struct {
	Path      string
	Content   []byte
	locations []depLocation
	changes   map[string]string // "section|name" -> new version spec
}

func FindPackageFiles(root string, recursive bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(root), packageFileName) {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a directory or package.json", root)
	}

	var files []string
	if !recursive {
		path := filepath.Join(root, packageFileName)
		if _, err := os.Stat(path); err == nil {
			return []string{path}, nil
		}
		return nil, fmt.Errorf("package.json not found in %s", root)
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == packageFileName {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func ParsePackageFile(path string) (*PackageFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pkg := &PackageFile{
		Path:    path,
		Content: append([]byte(nil), data...),
		changes: make(map[string]string),
	}
	if err := pkg.index(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return pkg, nil
}

func (p *PackageFile) index() error {
	dec := json.NewDecoder(bytes.NewReader(p.Content))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected object")
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected object key")
		}

		switch key {
		case "dependencies", "devDependencies":
			if err := p.indexDepSection(dec, key); err != nil {
				return err
			}
		default:
			if err := skipValue(dec); err != nil {
				return err
			}
		}
	}

	_, err = dec.Token()
	return err
}

func (p *PackageFile) indexDepSection(dec *json.Decoder, section string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("%s: expected object", section)
	}

	for dec.More() {
		nameTok, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := nameTok.(string)
		if !ok {
			return fmt.Errorf("%s: expected dependency name", section)
		}

		nameEnd := int(dec.InputOffset())
		valueStart, valueEnd, err := jsonStringValueBounds(p.Content, nameEnd)
		if err != nil {
			return fmt.Errorf("%s: dependency %q: %w", section, name, err)
		}

		valueTok, err := dec.Token()
		if err != nil {
			return err
		}
		version, ok := valueTok.(string)
		if !ok {
			return fmt.Errorf("%s: dependency %q version must be a string", section, name)
		}

		p.locations = append(p.locations, depLocation{
			Section:    section,
			Name:       name,
			Version:    version,
			ValueStart: valueStart,
			ValueEnd:   valueEnd,
		})
	}

	_, err = dec.Token()
	return err
}

func jsonStringValueBounds(content []byte, after int) (start, end int, err error) {
	i := after
	for i < len(content) && (content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r') {
		i++
	}
	if i >= len(content) || content[i] != ':' {
		return 0, 0, fmt.Errorf("expected ':' after key")
	}
	i++
	for i < len(content) && (content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r') {
		i++
	}
	if i >= len(content) || content[i] != '"' {
		return 0, 0, fmt.Errorf("expected string value")
	}
	start = i
	i++
	for i < len(content) {
		if content[i] == '\\' {
			i += 2
			continue
		}
		if content[i] == '"' {
			return start, i + 1, nil
		}
		i++
	}
	return 0, 0, fmt.Errorf("unterminated string value")
}

func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			for dec.More() {
				if _, err := dec.Token(); err != nil {
					return err
				}
				if err := skipValue(dec); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := skipValue(dec); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		default:
			return nil
		}
	default:
		return nil
	}
}

func (p *PackageFile) Dependencies(sections []string) ([]Dependency, error) {
	if len(sections) == 0 {
		sections = dependencySections
	}
	allowed := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		allowed[section] = struct{}{}
	}

	var deps []Dependency
	for _, loc := range p.locations {
		if _, ok := allowed[loc.Section]; !ok {
			continue
		}
		deps = append(deps, Dependency{
			Name:    loc.Name,
			Version: loc.Version,
			Section: loc.Section,
		})
	}
	return deps, nil
}

func (p *PackageFile) UpgradeableDependencies(sections []string, only string) ([]Dependency, error) {
	deps, err := p.Dependencies(sections)
	if err != nil {
		return nil, err
	}
	only = strings.TrimSpace(only)
	var out []Dependency
	for _, dep := range deps {
		if only != "" && dep.Name != only {
			continue
		}
		if !IsRegistryVersionSpec(dep.Version) {
			continue
		}
		out = append(out, dep)
	}
	return out, nil
}

func (p *PackageFile) SetDependency(section, name, version string) error {
	if section != "dependencies" && section != "devDependencies" {
		return fmt.Errorf("refusing to modify section %q", section)
	}
	for _, loc := range p.locations {
		if loc.Section == section && loc.Name == name {
			p.changes[changeKey(section, name)] = version
			return nil
		}
	}
	return fmt.Errorf("dependency %q not found in %s", name, section)
}

func changeKey(section, name string) string {
	return section + "|" + name
}

type depPatch struct {
	start int
	end   int
	value []byte
}

func (p *PackageFile) Save() error {
	if len(p.changes) == 0 {
		return nil
	}

	var patches []depPatch
	for _, loc := range p.locations {
		if v, ok := p.changes[changeKey(loc.Section, loc.Name)]; ok {
			encoded, err := json.Marshal(v)
			if err != nil {
				return err
			}
			patches = append(patches, depPatch{
				start: loc.ValueStart,
				end:   loc.ValueEnd,
				value: encoded,
			})
		}
	}
	if len(patches) == 0 {
		return nil
	}

	sortPatchesDesc(patches)
	content := append([]byte(nil), p.Content...)
	for _, item := range patches {
		content = append(content[:item.start], append(item.value, content[item.end:]...)...)
	}
	return os.WriteFile(p.Path, content, 0o644)
}

func sortPatchesDesc(patches []depPatch) {
	for i := 1; i < len(patches); i++ {
		j := i
		for j > 0 && patches[j-1].start < patches[j].start {
			patches[j-1], patches[j] = patches[j], patches[j-1]
			j--
		}
	}
}
