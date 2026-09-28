package application

import "testing"

func TestVersionNewerForkRevision(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		newer     bool
	}{
		{candidate: "1.2.15", current: "1.2.15-fork.1", newer: false},
		{candidate: "1.2.15-fork.1", current: "1.2.15", newer: true},
		{candidate: "1.2.15-fork.2", current: "1.2.15-fork.1", newer: true},
		{candidate: "v1.2.15-fork.2", current: "1.2.15-fork.1", newer: true},
		{candidate: "1.2.15-fork.10", current: "1.2.15-fork.2", newer: true},
		{candidate: "1.2.15-fork.1", current: "1.2.15-fork.1", newer: false},
		{candidate: "1.2.16", current: "1.2.15-fork.9", newer: true},
		{candidate: "1.2.16-rc.1", current: "1.2.15-fork.1", newer: true},
		{candidate: "1.2.16-rc.1", current: "1.2.16", newer: false},
		{candidate: "1.2.16", current: "1.2.16-rc.1", newer: true},
		{candidate: "1.2.16-rc.10", current: "1.2.16-rc.2", newer: true},
	}

	for _, tc := range cases {
		if got := versionNewer(tc.candidate, tc.current); got != tc.newer {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.newer)
		}
	}
}

func TestSelectUpdate(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		official string
		fork     string
		latest   string
		source   string
	}{
		{
			name:     "same official is not newer than fork",
			current:  "1.2.15-fork.1",
			official: "1.2.15",
			fork:     "1.2.15-fork.1",
			latest:   "1.2.15-fork.1",
			source:   "",
		},
		{
			name:     "newer fork revision",
			current:  "1.2.15-fork.1",
			official: "1.2.15",
			fork:     "1.2.15-fork.2",
			latest:   "1.2.15-fork.2",
			source:   sourceFork,
		},
		{
			name:     "official ahead of fork line",
			current:  "1.2.15-fork.1",
			official: "1.2.16",
			fork:     "1.2.15-fork.2",
			latest:   "1.2.16",
			source:   sourceOfficial,
		},
		{
			name:     "fork of the newer official release",
			current:  "1.2.15-fork.1",
			official: "1.2.16",
			fork:     "1.2.16-fork.1",
			latest:   "1.2.16-fork.1",
			source:   sourceFork,
		},
		{
			name:     "stock install can move to fork",
			current:  "1.2.15",
			official: "1.2.15",
			fork:     "1.2.15-fork.1",
			latest:   "1.2.15-fork.1",
			source:   sourceFork,
		},
		{
			name:     "older fork is not a downgrade",
			current:  "1.2.16",
			official: "1.2.16",
			fork:     "1.2.15-fork.3",
			latest:   "1.2.16",
			source:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			latest, source := selectUpdate(tc.current, tc.official, tc.fork)
			if latest != tc.latest || source != tc.source {
				t.Fatalf("selectUpdate() = %q, %q; want %q, %q", latest, source, tc.latest, tc.source)
			}
		})
	}
}

func TestNewestForkRelease(t *testing.T) {
	releases := []ghRelease{
		{
			TagName: "v1.2.15-fork.1",
			Assets: []ghAsset{{
				Name:               "nanokvm-pro-1.2.15-fork.1.tar.gz",
				BrowserDownloadURL: "https://example/1",
				Digest:             "sha256:aaa",
			}},
		},
		{
			TagName: "v1.2.15-fork.2",
			Assets: []ghAsset{{
				Name:               "nanokvm-pro-1.2.15-fork.2.tar.gz",
				BrowserDownloadURL: "https://example/2",
				Digest:             "sha256:bbb",
			}},
		},
		{
			TagName: "v1.2.15-fork.3",
			Draft:   true,
			Assets: []ghAsset{{
				Name:               "nanokvm-pro-1.2.15-fork.3.tar.gz",
				BrowserDownloadURL: "https://example/3",
			}},
		},
	}

	got := newestForkRelease(releases)
	if got == nil || got.Version != "1.2.15-fork.2" || got.URL != "https://example/2" || got.SHA256 != "bbb" {
		t.Fatalf("newestForkRelease() = %+v", got)
	}
}
