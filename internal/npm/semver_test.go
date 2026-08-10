package npm

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.1.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.2.0", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
	}
	for _, tc := range cases {
		got := CompareSemver(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("CompareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestResolveSpecVersion(t *testing.T) {
	cases := []struct {
		spec string
		want string
	}{
		{"^1.2.3", "1.2.3"},
		{"~2.0.0", "2.0.0"},
		{"1.x", "1.0.0"},
		{"*", "0.0.0"},
		{"18.3.1", "18.3.1"},
	}
	for _, tc := range cases {
		got, ok := ResolveSpecVersion(tc.spec)
		if !ok || got != tc.want {
			t.Fatalf("ResolveSpecVersion(%q) = (%q, %v), want %q", tc.spec, got, ok, tc.want)
		}
	}
}

func TestUpgradeSpec(t *testing.T) {
	cases := []struct {
		spec, target, want string
	}{
		{"^1.2.3", "2.0.0", "^2.0.0"},
		{"~1.2.3", "1.3.0", "~1.3.0"},
		{"1.2.3", "1.3.0", "1.3.0"},
		{"1.x", "2.1.0", "2.x"},
		{"*", "2.0.0", "*"},
	}
	for _, tc := range cases {
		got := UpgradeSpec(tc.spec, tc.target)
		if got != tc.want {
			t.Fatalf("UpgradeSpec(%q, %q) = %q, want %q", tc.spec, tc.target, got, tc.want)
		}
	}
}

func TestSelectTargetVersionLatest(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0", "2.0.0-rc1"}
	opts := DefaultTargetOptions(TargetLatest)
	target, ok := SelectTargetVersion("1.0.0", available, TargetLatest, opts)
	if !ok || target != "2.0.0" {
		t.Fatalf("got (%q, %v), want (2.0.0, true)", target, ok)
	}
}
