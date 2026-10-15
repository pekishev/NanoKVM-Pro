package hid

import (
	"reflect"
	"testing"
)

func TestParseScriptCommands(t *testing.T) {
	commands, line, err := parseScript(`# comment
REM ignored
DELAY 500
STRING Hello
STRINGLN world
MOVE 960 540
CLICK 371 67 RIGHT
DBLCLICK 10 20
WIN+SHIFT+S
CTRL+ALT+DELETE
ALT F4
ENTER
RUN Other
WAITIMAGE boot-ok 20000
`)
	if err != nil {
		t.Fatalf("parse: %v (line %d)", err, line)
	}

	want := []scriptCommand{
		{Type: "delay", Ms: 500},
		{Type: "string", Text: "Hello"},
		{Type: "string", Text: "world\n"},
		{Type: "move", X: 960, Y: 540},
		{Type: "click", X: 371, Y: 67, Button: "right", Times: 1},
		{Type: "click", X: 10, Y: 20, Button: "left", Times: 2},
		{Type: "key", Codes: []string{"MetaLeft", "ShiftLeft", "KeyS"}},
		{Type: "key", Codes: []string{"ControlLeft", "AltLeft", "Delete"}},
		{Type: "key", Codes: []string{"AltLeft", "F4"}},
		{Type: "key", Codes: []string{"Enter"}},
		{Type: "run", Name: "Other"},
		{Type: "waitimage", Name: "boot-ok", Ms: 20000},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("got %#v\nwant %#v", commands, want)
	}
}

func TestParseScriptRejectsBadLines(t *testing.T) {
	cases := []struct {
		script string
		line   int
	}{
		{"", 1},
		{"# only comments", 1},
		{"DELAY", 1},
		{"DELAY 70000", 1},
		{"STRING", 1},
		{"CLICK 5000 10", 1},
		{"CLICK 10.5 20", 1},
		{"WAITIMAGE bad name 1000", 1},
		{"WAITIMAGE boot 1000 extra", 1},
		{"FOO", 1},
		{"DELAY 1\nUNKNOWNKEY", 2},
	}
	for _, tc := range cases {
		_, line, err := parseScript(tc.script)
		if err == nil {
			t.Fatalf("%q: expected error", tc.script)
		}
		if line != tc.line {
			t.Fatalf("%q: line %d want %d (%v)", tc.script, line, tc.line, err)
		}
	}
}

func TestParseQuotedStringAndRun(t *testing.T) {
	commands, _, err := parseScript("STRING \"Hello World\"\nRUN 'Inner Macro'")
	if err != nil {
		t.Fatal(err)
	}
	if commands[0].Text != "Hello World" {
		t.Fatalf("string = %q", commands[0].Text)
	}
	if commands[1].Name != "Inner Macro" {
		t.Fatalf("run = %q", commands[1].Name)
	}
}
