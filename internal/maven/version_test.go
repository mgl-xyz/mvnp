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
		{"2.0.64", "2.0.64.android8", 1},
		{"2.0.64.android8", "2.0.64", -1},
		{"2.0.64.android8", "2.0.65.android8", -1},
		{"1.0", "1.0-alpha", 1},
		{"1.0", "1.0.1", -1},
		{"4.1.100.Final", "4.1.108.Final", -1},
	}
	for _, tc := range cases {
		got := CompareVersions(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestVersionFamily(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"2.0.64", ""},
		{"2.0.64-SNAPSHOT", ""},
		{"2.0.64.Final", ""},
		{"4.1.108.Final", ""},
		{"1.0-GA", ""},
		{"1.0-sp1", ""},
		{"3.4.2.v20210922", ""},
		{"2.0.64.android8", "android8"},
		{"2.0.64.android5", "android5"},
		{"2.0.64.android8-SNAPSHOT", "android8"},
		{"2.0.2.android", "android"},
		{"32.1.3-jre", "jre"},
		{"32.1.3-android", "android"},
		{"4.0.15-indy", "indy"},
	}
	for _, tc := range cases {
		if got := VersionFamily(tc.version); got != tc.want {
			t.Fatalf("VersionFamily(%q) = %q, want %q", tc.version, got, tc.want)
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

func TestSelectVersionIgnoresPlatformSuffix(t *testing.T) {
	available := []string{
		"2.0.63",
		"2.0.64",
		"2.0.64.android5",
		"2.0.64.android8",
		"2.0.64-rc1",
	}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("2.0.50", available, PolicyLatestReleases, opts)
	if !ok || target != "2.0.64" {
		t.Fatalf("got (%q, %v), want (2.0.64, true)", target, ok)
	}
}

func TestSelectVersionKeepsSamePlatformFamily(t *testing.T) {
	available := []string{
		"2.0.63",
		"2.0.64",
		"2.0.63.android8",
		"2.0.64.android5",
		"2.0.64.android8",
	}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("2.0.50.android8", available, PolicyLatestReleases, opts)
	if !ok || target != "2.0.64.android8" {
		t.Fatalf("got (%q, %v), want (2.0.64.android8, true)", target, ok)
	}
}

func TestSelectVersionGuavaJreVsAndroid(t *testing.T) {
	available := []string{"32.1.3-jre", "32.1.3-android", "33.0.0-jre", "33.0.0-android"}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("32.0.0-jre", available, PolicyLatestReleases, opts)
	if !ok || target != "33.0.0-jre" {
		t.Fatalf("got (%q, %v), want (33.0.0-jre, true)", target, ok)
	}
}

func TestSelectVersionNettyFinal(t *testing.T) {
	available := []string{"4.1.100.Final", "4.1.108.Final", "4.2.0.Final"}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("4.1.100.Final", available, PolicyLatestReleases, opts)
	if !ok || target != "4.2.0.Final" {
		t.Fatalf("got (%q, %v), want (4.2.0.Final, true)", target, ok)
	}
}

func TestSelectVersionAlreadyLatestIgnoresNewerSuffix(t *testing.T) {
	available := []string{"2.0.64", "2.0.64.android8"}
	opts := DefaultPolicyOptions(PolicyLatestReleases)
	target, ok := SelectVersion("2.0.64", available, PolicyLatestReleases, opts)
	if ok {
		t.Fatalf("got (%q, %v), want no upgrade", target, ok)
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
