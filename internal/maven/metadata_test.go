package maven

import (
	"path/filepath"
	"testing"
)

func TestCachingRepositoryDiskCache(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	repo := NewCachingRepository("", cacheDir)

	versions := []string{"1.0.0", "2.0.0"}
	repo.saveDiskCache("com.example:demo", versions)

	loaded, err := repo.loadDiskCache("com.example:demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[1] != "2.0.0" {
		t.Fatalf("loaded %#v", loaded)
	}

	repo.storeMemory("com.example:demo", versions, nil)
	got, err := repo.ListVersions("com.example", "demo")
	if err != nil || len(got) != 2 {
		t.Fatalf("ListVersions from cache failed: %v %#v", err, got)
	}
}
