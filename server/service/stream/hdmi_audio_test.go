package stream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHdmiAudioFlagDefaultsOff(t *testing.T) {
	HdmiAudioFlag = filepath.Join(t.TempDir(), "hdmi-audio")
	t.Cleanup(func() { HdmiAudioFlag = "/etc/kvm/hdmi-audio" })

	if IsHdmiAudioEnabled() {
		t.Fatal("HDMI audio should be off when the flag file is missing")
	}

	if err := SetHdmiAudioEnabled(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !IsHdmiAudioEnabled() {
		t.Fatal("HDMI audio should be on after creating the flag file")
	}

	if err := SetHdmiAudioEnabled(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if IsHdmiAudioEnabled() {
		t.Fatal("HDMI audio should be off after removing the flag file")
	}
	if _, err := os.Stat(HdmiAudioFlag); !os.IsNotExist(err) {
		t.Fatalf("flag file still present: %v", err)
	}
}
