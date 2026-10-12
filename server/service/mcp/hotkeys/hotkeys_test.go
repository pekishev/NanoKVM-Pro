package hotkeys

import "testing"

func TestPriorityCombos(t *testing.T) {
	cases := map[string]Chord{
		"Ctrl+C":         {0x01, 0x06, "Ctrl+C"},
		"Ctrl+L":         {0x01, 0x0F, "Ctrl+L"},
		"Ctrl+U":         {0x01, 0x18, "Ctrl+U"},
		"Alt+Tab":        {0x04, 0x2B, "Alt+Tab"},
		"Backspace":      {0x00, 0x2A, "Backspace"},
		"Delete":         {0x00, 0x4C, "Delete"},
		"Win+E":          {0x08, 0x08, "Win+E"},
		"Ctrl+Shift+Esc": {0x01 | 0x02, 0x29, "Ctrl+Shift+Esc"},
	}
	for text, want := range cases {
		got, err := ParseChord(text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if got != want {
			t.Fatalf("%s: got %+v want %+v", text, got, want)
		}
	}
}

func TestSequenceAndCommaKey(t *testing.T) {
	chords, err := ParseKeys("Ctrl+A, Delete")
	if err != nil {
		t.Fatal(err)
	}
	if len(chords) != 2 || chords[0].Label != "Ctrl+A" || chords[1].Label != "Delete" {
		t.Fatalf("got %+v", chords)
	}
	comma, err := ParseChord("Ctrl+,")
	if err != nil {
		t.Fatal(err)
	}
	if comma.Label != "Ctrl+," || comma.Keycode != 0x36 {
		t.Fatalf("got %+v", comma)
	}
}

func TestUnknownKey(t *testing.T) {
	if _, err := ParseChord("Ctrl+Nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestVirtualKeyboardRow(t *testing.T) {
	for _, name := range []string{"Esc", "F12", "Tab", "Enter", "Space", "CapsLock", "Up", "PageDown", "Menu"} {
		chord, err := ParseChord(name)
		if err != nil || chord.Keycode == 0 {
			t.Fatalf("%s: %+v %v", name, chord, err)
		}
	}
}
