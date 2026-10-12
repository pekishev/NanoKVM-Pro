package layout

import (
	"strconv"
	"strings"
	"testing"
)

func TestEnglishStaysOnEnglish(t *testing.T) {
	segments := Plan("Hello, world!", EN)
	if len(segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(segments))
	}
	if segments[0].Layout != EN || segments[0].Keys != "Hello, world!" {
		t.Fatalf("got %+v", segments[0])
	}
}

func TestRussianFromEnglishSwitchesOnce(t *testing.T) {
	segments := Plan("привет", EN)
	if len(segments) != 1 || segments[0].Layout != RU || segments[0].Keys != "ghbdtn" {
		t.Fatalf("got %+v", segments)
	}
}

func TestCapitalRussianUsesShift(t *testing.T) {
	segments := Plan("Я", EN)
	if segments[0].Keys != "Z" {
		t.Fatalf("Я → %q", segments[0].Keys)
	}
	if Plan("Ё", RU)[0].Keys != "~" {
		t.Fatalf("Ё → %q", Plan("Ё", RU)[0].Keys)
	}
}

func TestMixedTextSplitsOnLayout(t *testing.T) {
	segments := Plan("hi мир", EN)
	got := joinSeg(segments)
	want := "en:hi |ru:vbh"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSpaceDoesNotLeaveRussian(t *testing.T) {
	segments := Plan("привет мир", EN)
	if len(segments) != 1 || segments[0].Layout != RU || segments[0].Keys != "ghbdtn vbh" {
		t.Fatalf("got %+v", segments)
	}
}

func TestRussianPunctuationStaysOnRussian(t *testing.T) {
	segments := Plan("а,б", RU)
	if segments[0].Layout != RU || segments[0].Keys != "f?," {
		t.Fatalf("got %+v", segments[0])
	}
}

func TestDigitsDoNotSwitch(t *testing.T) {
	segments := Plan("12", RU)
	if len(segments) != 1 || segments[0].Layout != RU || segments[0].Keys != "12" {
		t.Fatalf("got %+v", segments)
	}
}

func TestCRIsSkippedAndNewlineIsEnter(t *testing.T) {
	segments := Plan("a\r\nb", EN)
	if segments[0].Keys != "a\nb" {
		t.Fatalf("keys = %q", segments[0].Keys)
	}
	if fmtInts(segments[0].Src) != "0,2,3" {
		t.Fatalf("src = %v", segments[0].Src)
	}
}

func TestEveryPlannedKeyHasHIDCode(t *testing.T) {
	text := "Hello, мир!\n\t0123 №"
	for _, segment := range Plan(text, EN) {
		for _, ch := range segment.Keys {
			_, code, ok := USHID(ch)
			if !ok || code < 4 {
				t.Fatalf("no HID for %q", ch)
			}
		}
	}
}

func TestUSHIDMatchesPasteMap(t *testing.T) {
	cases := map[rune][2]byte{
		'a':  {0, 4},
		'A':  {2, 4},
		'1':  {0, 30},
		'!':  {2, 30},
		'\n': {0, 40},
		' ':  {0, 44},
		'"':  {2, 52},
		'\'': {0, 52},
	}
	for ch, want := range cases {
		mod, code, ok := USHID(ch)
		if !ok || mod != want[0] || code != want[1] {
			t.Fatalf("%q = %d,%d ok=%v want %v", ch, mod, code, ok, want)
		}
	}
}

func TestASCIIQuotesStayOnEnglish(t *testing.T) {
	segments := Plan(`grep "Failed"`, EN)
	if segments[0].Keys != `grep "Failed"` {
		t.Fatalf("got %q", segments[0].Keys)
	}
}

func joinSeg(segments []Segment) string {
	parts := make([]string, 0, len(segments))
	for _, s := range segments {
		parts = append(parts, string(s.Layout)+":"+s.Keys)
	}
	return strings.Join(parts, "|")
}

func fmtInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
