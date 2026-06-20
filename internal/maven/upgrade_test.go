package maven

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type stubRepository struct {
	versions map[string][]string
	err      error
}

func (s *stubRepository) ListVersions(groupID, artifactID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	key := ArtifactCoordinate(groupID, artifactID)
	versions, ok := s.versions[key]
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", key)
	}
	return versions, nil
}

func TestUpgradePropertyReferencedVersion(t *testing.T) {
	dir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.example</groupId>
  <artifactId>demo</artifactId>
  <version>1.0.0</version>
  <properties>
    <jackson2.version>2.15.2</jackson2.version>
    <maven-compiler-plugin.version>3.8.1</maven-compiler-plugin.version>
  </properties>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.fasterxml.jackson</groupId>
        <artifactId>jackson-bom</artifactId>
        <version>${jackson2.version}</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-compiler-plugin</artifactId>
        <version>${maven-compiler-plugin.version}</version>
      </plugin>
    </plugins>
  </build>
</project>`
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(pomContent), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRepository{versions: map[string][]string{
		"com.fasterxml.jackson:jackson-bom":        {"2.15.2", "2.16.0", "2.17.0"},
		"org.apache.maven.plugins:maven-compiler-plugin": {"3.8.1", "3.11.0", "3.12.1"},
	}}

	report, err := Upgrade(UpgradeRequest{
		Root:         dir,
		Policy:       PolicyLatestReleases,
		Repository:   repo,
		AutoBackup:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed != 2 {
		t.Fatalf("expected 2 upgrades, got %d", report.Changed)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(updated)
	if !contains(content, "<jackson2.version>2.17.0</jackson2.version>") {
		t.Fatalf("expected jackson2.version upgraded: %s", content)
	}
	if !contains(content, "<maven-compiler-plugin.version>3.12.1</maven-compiler-plugin.version>") {
		t.Fatalf("expected maven-compiler-plugin.version upgraded: %s", content)
	}
	if contains(content, "<version>2.17.0</version>") {
		t.Fatalf("dependency version tag should remain property reference: %s", content)
	}
}

func TestUpgradeSharedPropertyReportsAllDependents(t *testing.T) {
	dir := t.TempDir()
	pomContent := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <properties>
    <jackson2.version>2.15.2</jackson2.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>com.fasterxml.jackson</groupId>
      <artifactId>jackson-bom</artifactId>
      <version>${jackson2.version}</version>
      <type>pom</type>
      <scope>import</scope>
    </dependency>
  </dependencies>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.fasterxml.jackson</groupId>
        <artifactId>jackson-bom</artifactId>
        <version>${jackson2.version}</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
</project>`
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(pomContent), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRepository{versions: map[string][]string{
		"com.fasterxml.jackson:jackson-bom": {"2.15.2", "2.17.0"},
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
	if report.Changed != 2 {
		t.Fatalf("expected 2 reported upgrades, got %d", report.Changed)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if countSubstring(string(updated), "2.17.0") != 1 {
		t.Fatalf("property value should be written once: %s", updated)
	}
}

func countSubstring(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}
