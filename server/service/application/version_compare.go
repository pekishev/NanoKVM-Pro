package application

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	sourceOfficial = "official"
	sourceFork     = "fork"
)

var (
	versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z][0-9A-Za-z.-]*))?(?:\+[0-9A-Za-z.-]+)?$`)
	forkPattern    = regexp.MustCompile(`^fork\.(\d+)$`)
)

type appVersion struct {
	major int
	minor int
	patch int
	pre   string
	fork  int
	ok    bool
}

func parseAppVersion(raw string) appVersion {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return appVersion{}
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	pre := match[4]
	fork := 0
	if revision, ok := forkRevision(pre); ok {
		fork = revision
		pre = ""
	}

	return appVersion{
		major: major,
		minor: minor,
		patch: patch,
		pre:   pre,
		fork:  fork,
		ok:    true,
	}
}

func forkRevision(pre string) (int, bool) {
	match := forkPattern.FindStringSubmatch(pre)
	if match == nil {
		return 0, false
	}

	revision, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}

	return revision, true
}

func versionNewer(candidate, current string) bool {
	next := parseAppVersion(candidate)
	installed := parseAppVersion(current)
	if !next.ok || !installed.ok {
		return false
	}

	return compareApp(next, installed) > 0
}

func compareApp(a, b appVersion) int {
	if cmp := compareInt(a.major, b.major); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(a.minor, b.minor); cmp != 0 {
		return cmp
	}
	if cmp := compareInt(a.patch, b.patch); cmp != 0 {
		return cmp
	}
	if cmp := comparePre(a.pre, b.pre); cmp != 0 {
		return cmp
	}

	return compareInt(a.fork, b.fork)
}

func compareBase(a, b appVersion) int {
	a.fork = 0
	b.fork = 0
	return compareApp(a, b)
}

func comparePre(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}

	left := strings.Split(a, ".")
	right := strings.Split(b, ".")
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}

	for i := 0; i < limit; i++ {
		leftNum, leftErr := strconv.Atoi(left[i])
		rightNum, rightErr := strconv.Atoi(right[i])
		switch {
		case leftErr == nil && rightErr == nil:
			if cmp := compareInt(leftNum, rightNum); cmp != 0 {
				return cmp
			}
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			if cmp := strings.Compare(left[i], right[i]); cmp != 0 {
				return cmp
			}
		}
	}

	return compareInt(len(left), len(right))
}

func compareInt(a, b int) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func selectUpdate(current, official, fork string) (string, string) {
	forkNewer := fork != "" && versionNewer(fork, current)
	officialNewer := official != "" && versionNewer(official, current)

	switch {
	case forkNewer && officialNewer && fork == official:
		// A fork release published in the official package format is preferred,
		// because dpkg then tracks the installed files.
		return official, sourceOfficial
	case forkNewer && officialNewer:
		if compareBase(parseAppVersion(fork), parseAppVersion(official)) >= 0 {
			return fork, sourceFork
		}
		return official, sourceOfficial
	case forkNewer:
		return fork, sourceFork
	case officialNewer:
		return official, sourceOfficial
	default:
		return current, ""
	}
}
