package mcp

import "testing"

func TestTapSequencePlusSplitsShiftFromKey(t *testing.T) {
	seq := tapSequence(2, 46)
	want := []hidTap{
		{2, 0, hidSettle},
		{2, 46, keyHold},
		{2, 0, hidSettle},
		{0, 0, hidSettle + keyGap},
	}
	if len(seq) != len(want) {
		t.Fatalf("len=%d want %d: %+v", len(seq), len(want), seq)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("step %d: %+v want %+v", i, seq[i], want[i])
		}
	}
}

func TestTapSequenceMinusIsUnshifted(t *testing.T) {
	seq := tapSequence(0, 45)
	if len(seq) != 2 || seq[0] != (hidTap{0, 45, keyHold}) || seq[1] != (hidTap{0, 0, keyGap}) {
		t.Fatalf("got %+v", seq)
	}
}

func TestTapSequenceNeverChangesModifierAndKeyTogether(t *testing.T) {
	pairs := [][2]byte{{2, 46}, {0, 45}, {2, 4}, {0, 4}}
	for _, pair := range pairs {
		seq := tapSequence(pair[0], pair[1])
		for i := 1; i < len(seq); i++ {
			a, b := seq[i-1], seq[i]
			if a.modifier != b.modifier && (a.keycode != 0 || b.keycode != 0) {
				t.Fatalf("%v step %d: modifier and key both change %+v → %+v", pair, i, a, b)
			}
		}
	}
}
