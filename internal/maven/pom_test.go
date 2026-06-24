package maven

import (
	"os"
	"path/filepath"
	"testing"
)

const samplePOM = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.example</groupId>
  <artifactId>demo</artifactId>
  <version>1.0.0</version>
  <properties>
    <junit.version>5.9.0</junit.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-lang3</artifactId>
      <version>3.12.0</version>
    </dependency>
    <dependency>
      <groupId>org.junit.jupiter</groupId>
      <artifactId>junit-jupiter</artifactId>
      <version>${junit.version}</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-compiler-plugin</artifactId>
        <version>3.8.1</version>
      </plugin>
    </plugins>
  </build>
</project>
`

func TestParsePOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}

	pom, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pom.Deps) < 2 {
		t.Fatalf("expected at least 2 dependencies/plugins, got %d", len(pom.Deps))
	}
	if got := pom.ResolveVersion("${junit.version}"); got != "5.9.0" {
		t.Fatalf("resolved property version = %q, want 5.9.0", got)
	}
}

func TestApplyVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}

	pom, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}

	var target *Dependency
	for i := range pom.Deps {
		if pom.Deps[i].ArtifactID == "commons-lang3" {
			target = &pom.Deps[i]
			break
		}
	}
	if target == nil {
		t.Fatal("commons-lang3 dependency not found")
	}

	pom.ApplyVersion(*target, "3.14.0")
	if err := pom.Save(); err != nil {
		t.Fatal(err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(updated), "<version>3.14.0</version>") {
		t.Fatalf("expected updated version in pom: %s", updated)
	}
}

func TestApplyPropertyVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}

	pom, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := pom.PropertyByKey("junit.version")
	if !ok {
		t.Fatal("junit.version property not found")
	}

	pom.ApplyPropertyVersion(entry, "5.10.2")
	if err := pom.Save(); err != nil {
		t.Fatal(err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(updated), "<junit.version>5.10.2</junit.version>") {
		t.Fatalf("expected updated property in pom: %s", updated)
	}
	if contains(string(updated), "<version>5.10.2</version>") {
		t.Fatalf("property upgrade should not rewrite dependency version tag: %s", updated)
	}
}

func TestUpgradeableDependenciesRequireVersion(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
  </parent>
  <dependencies>
    <dependency>
      <groupId>org.junit.jupiter</groupId>
      <artifactId>junit-jupiter</artifactId>
      <scope>test</scope>
    </dependency>
    <dependency>
      <groupId>org.apache.commons</groupId>
      <artifactId>commons-lang3</artifactId>
      <version>3.12.0</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-surefire-plugin</artifactId>
      </plugin>
    </plugins>
  </build>
</project>`
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pom, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}
	if pom.Parent != nil {
		t.Fatal("parent without version should not be parsed")
	}

	deps := pom.UpgradeableDependencies(true, "")
	if len(deps) != 1 {
		t.Fatalf("expected 1 upgradeable dependency, got %d: %+v", len(deps), deps)
	}
	if deps[0].ArtifactID != "commons-lang3" {
		t.Fatalf("unexpected upgradeable artifact: %+v", deps[0])
	}
}

func TestParsePluginAnnotationProcessorPath(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <therapi-javadoc.version>0.15.0</therapi-javadoc.version>
    <java.version>17</java.version>
  </properties>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-compiler-plugin</artifactId>
        <configuration>
          <source>${java.version}</source>
          <target>${java.version}</target>
          <annotationProcessorPaths>
            <path>
              <groupId>com.github.therapi</groupId>
              <artifactId>therapi-runtime-javadoc-scribe</artifactId>
              <version>${therapi-javadoc.version}</version>
            </path>
          </annotationProcessorPaths>
        </configuration>
      </plugin>
    </plugins>
  </build>
</project>`
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pom, err := ParsePOM(path)
	if err != nil {
		t.Fatal(err)
	}

	var compilerPlugin, therapiPath *Dependency
	for i := range pom.Deps {
		switch pom.Deps[i].ArtifactID {
		case "maven-compiler-plugin":
			compilerPlugin = &pom.Deps[i]
		case "therapi-runtime-javadoc-scribe":
			therapiPath = &pom.Deps[i]
		}
	}
	if compilerPlugin != nil {
		t.Fatalf("unexpected compiler plugin entry: %+v", *compilerPlugin)
	}
	if therapiPath == nil {
		t.Fatal("therapi annotation processor path not found")
	}
	if therapiPath.GroupID != "com.github.therapi" {
		t.Fatalf("groupId = %q", therapiPath.GroupID)
	}
	if therapiPath.Version != "${therapi-javadoc.version}" {
		t.Fatalf("version = %q", therapiPath.Version)
	}
	if therapiPath.Section != "plugins:path" {
		t.Fatalf("section = %q", therapiPath.Section)
	}
}

func TestUpgradeAnnotationProcessorPathProperty(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <therapi-javadoc.version>0.14.0</therapi-javadoc.version>
  </properties>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-compiler-plugin</artifactId>
        <configuration>
          <annotationProcessorPaths>
            <path>
              <groupId>com.github.therapi</groupId>
              <artifactId>therapi-runtime-javadoc-scribe</artifactId>
              <version>${therapi-javadoc.version}</version>
            </path>
          </annotationProcessorPaths>
        </configuration>
      </plugin>
    </plugins>
  </build>
</project>`
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRepository{versions: map[string][]string{
		"com.github.therapi:therapi-runtime-javadoc-scribe": {"0.14.0", "0.15.0"},
		"org.apache.maven.plugins:maven-compiler-plugin":    {"3.13.0", "3.15.0"},
	}}

	report, err := Upgrade(UpgradeRequest{
		Root:       dir,
		Policy:     PolicyLatestReleases,
		Repository: repo,
		AutoBackup: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, result := range report.Results {
		if result.ArtifactID == "maven-compiler-plugin" && !result.Skipped {
			t.Fatalf("unexpected compiler plugin upgrade: %+v", result)
		}
	}

	var therapiResult *UpgradeResult
	for i := range report.Results {
		if report.Results[i].ArtifactID == "therapi-runtime-javadoc-scribe" {
			therapiResult = &report.Results[i]
			break
		}
	}
	if therapiResult == nil {
		t.Fatal("therapi upgrade result not found")
	}
	if therapiResult.Skipped {
		t.Fatalf("therapi unexpectedly skipped: %s", therapiResult.Reason)
	}
	if therapiResult.NewVersion != "0.15.0" {
		t.Fatalf("therapi new version = %q, want 0.15.0", therapiResult.NewVersion)
	}
}

func TestWritablePropertyChain(t *testing.T) {
	content := `<project><properties>
    <jackson.version>${jackson-bom.version}</jackson.version>
    <jackson-bom.version>2.15.2</jackson-bom.version>
  </properties></project>`
	pom := &POM{Content: content}
	pom.PropertyList, pom.Properties = parseProperties(content)

	entry, resolved, ok := pom.WritableProperty("jackson.version")
	if !ok {
		t.Fatal("expected writable property")
	}
	if entry.Key != "jackson-bom.version" {
		t.Fatalf("writable key = %q, want jackson-bom.version", entry.Key)
	}
	if resolved != "2.15.2" {
		t.Fatalf("resolved = %q, want 2.15.2", resolved)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
