package npm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	original := `{
  "name": "my-app",
  "version": "1.0.0",
  "private": true,
  "scripts": {
    "dev": "vite",
    "build": "vite build"
  },
  "dependencies": {
    "react": "^18.0.0",
    "lodash": "^4.17.20"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  },
  "engines": {
    "node": ">=18"
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	pkg, err := ParsePackageFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetDependency("dependencies", "react", "^18.3.1"); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetDependency("devDependencies", "vite", "^5.4.0"); err != nil {
		t.Fatal(err)
	}
	if err := pkg.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	for _, want := range []string{
		`"name": "my-app"`,
		`"version": "1.0.0"`,
		`"private": true`,
		`"scripts"`,
		`"dev": "vite"`,
		`"engines"`,
		`"node": ">=18"`,
		`"lodash": "^4.17.20"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing preserved content %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"react": "^18.3.1"`) {
		t.Fatalf("react not upgraded:\n%s", out)
	}
	if !strings.Contains(out, `"vite": "^5.4.0"`) {
		t.Fatalf("vite not upgraded:\n%s", out)
	}
}

func TestDoesNotTouchPeerOrOptionalByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	original := `{
  "dependencies": {
    "react": "^18.0.0"
  },
  "peerDependencies": {
    "react-dom": "^18.0.0"
  },
  "optionalDependencies": {
    "fsevents": "^2.3.0"
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &stubRegistry{versions: map[string][]string{
		"react":     {"18.0.0", "18.3.1"},
		"react-dom": {"18.0.0", "18.3.1"},
		"fsevents":  {"2.3.0", "2.3.3"},
	}}

	_, err := Upgrade(UpgradeRequest{
		Root:       dir,
		Target:     TargetLatest,
		Registry:   repo,
		AutoBackup: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, `"react-dom": "^18.0.0"`) {
		t.Fatalf("peerDependencies should be untouched:\n%s", out)
	}
	if !strings.Contains(out, `"fsevents": "^2.3.0"`) {
		t.Fatalf("optionalDependencies should be untouched:\n%s", out)
	}
	if !strings.Contains(out, `"react": "^18.3.1"`) {
		t.Fatalf("dependencies should be upgraded:\n%s", out)
	}
}
