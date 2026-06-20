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
