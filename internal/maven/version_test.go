package maven

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.1", -1},
		{"1.10", "1.2", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0-SNAPSHOT", "1.0", 0},
	}
	for _, tc := range cases {
		got := CompareVersions(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSelectVersionLatestReleases(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0-SNAPSHOT", "2.0.0-rc1"}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("1.0.0", available, PolicyLatestReleases, opts)
	if !ok || target != "1.2.0" {
		t.Fatalf("got (%q, %v), want (1.2.0, true)", target, ok)
	}
}

func TestSelectVersionReleases(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0-SNAPSHOT"}
	opts := DefaultPolicyOptions(PolicyReleases)
	target, ok := SelectVersion("2.0.0-SNAPSHOT", available, PolicyReleases, opts)
	if !ok || target != "1.1.0" {
		t.Fatalf("got (%q, %v), want (1.1.0, true)", target, ok)
	}
}

func TestSelectVersionNextReleases(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "1.2.0"}
	opts := DefaultPolicyOptions(PolicyNextReleases)
	target, ok := SelectVersion("1.0.0", available, PolicyNextReleases, opts)
	if !ok || target != "1.1.0" {
		t.Fatalf("got (%q, %v), want (1.1.0, true)", target, ok)
	}
}

func TestParsePolicyAliases(t *testing.T) {
	cases := map[string]Policy{
		"release":  PolicyLatestReleases,
		"stable":   PolicyLatestReleases,
		"snapshot": PolicyLatestSnapshots,
		"latest":   PolicyLatestVersions,
		"next":     PolicyNextVersions,
	}
	for input, want := range cases {
		got, err := ParsePolicy(input)
		if err != nil || got != want {
			t.Fatalf("ParsePolicy(%q) = (%q, %v), want %q", input, got, err, want)
		}
	}
}
