package stream

import "os"

// HdmiAudioFlag enables HDMI audio capture into the browser WebRTC session.
// The file is absent by default, matching Virtual Mic's opt-in USB gadget.
var HdmiAudioFlag = "/etc/kvm/hdmi-audio"

func IsHdmiAudioEnabled() bool {
	_, err := os.Stat(HdmiAudioFlag)
	return err == nil
}

func SetHdmiAudioEnabled(enabled bool) error {
	if enabled {
		file, err := os.OpenFile(HdmiAudioFlag, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		return file.Close()
	}

	if err := os.Remove(HdmiAudioFlag); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
