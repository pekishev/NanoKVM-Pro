package hid

import (
	"context"
	"encoding/base64"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"NanoKVM-Server/proto"
)

func TestWaitImageSpecFromReq(t *testing.T) {
	spec, err := WaitImageSpecFromReq(proto.WaitImageReq{Name: "boot-ok"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Ms != waitImageDefaultMS || spec.Name != "boot-ok" {
		t.Fatalf("%+v", spec)
	}

	zero := 0
	spec, err = WaitImageSpecFromReq(proto.WaitImageReq{Name: "boot-ok", Ms: &zero})
	if err != nil || spec.Ms != 0 {
		t.Fatalf("ms 0: %+v %v", spec, err)
	}

	tooBig := macroImageTimeoutMax + 1
	if _, err := WaitImageSpecFromReq(proto.WaitImageReq{Name: "boot-ok", Ms: &tooBig}); err == nil {
		t.Fatal("expected invalid timeout")
	}
	if _, err := WaitImageSpecFromReq(proto.WaitImageReq{}); err == nil {
		t.Fatal("expected missing image")
	}
	if _, err := WaitImageSpecFromReq(proto.WaitImageReq{Name: "bad name"}); err == nil {
		t.Fatal("expected invalid name")
	}
}

func TestPlayWaitImageFromPNG(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	frame := solidFrame(20, 20, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	pngBytes := mustPNGBytes(t, 4, 4, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	sleepFn = func(context.Context, time.Duration) error { return nil }
	grabFrameFn = func() (*rgbFrame, error) { return frame, nil }
	frameSizeFn = func() (int, int, error) { return 20, 20, nil }

	name, err := PlayWaitImage(context.Background(), WaitImageSpec{
		Png:    base64.StdEncoding.EncodeToString(pngBytes),
		X:      0,
		Y:      0,
		W:      4,
		H:      4,
		FrameW: 20,
		FrameH: 20,
		Ms:     1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "inline" {
		t.Fatalf("name = %s", name)
	}
}

func TestWaitImageTimesOut(t *testing.T) {
	t.Cleanup(restorePlayerHooks)
	live := solidFrame(20, 20, color.RGBA{R: 255, A: 255})
	ref := cropRef(solidFrame(20, 20, color.RGBA{R: 1, A: 255}), 0, 0, 4, 4)
	sleepFn = func(context.Context, time.Duration) error { return nil }
	grabFrameFn = func() (*rgbFrame, error) { return live, nil }

	err := waitForLoadedImage(context.Background(), ref, 0)
	if !errors.Is(err, ErrWaitImageTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveWaitImageByName(t *testing.T) {
	root := t.TempDir()
	previousRoot := macroImageRoot
	previousFile := macroFile
	macroImageRoot = root
	macroFile = filepath.Join(root, "macros.json")
	t.Cleanup(func() {
		macroImageRoot = previousRoot
		macroFile = previousFile
	})

	id := "11111111-1111-4111-8111-111111111111"
	if err := os.WriteFile(macroFile, []byte(`{"macros":[{"id":"`+id+`","name":"Boot","script":"ENTER"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	refPNG := mustPNGBytes(t, 4, 4, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	if err := writeMacroImage(id, "boot-ok", pixelMeta(0, 0, 4, 4, 20, 20), refPNG); err != nil {
		t.Fatal(err)
	}

	spec, err := WaitImageSpecFromReq(proto.WaitImageReq{Name: "boot-ok"})
	if err != nil {
		t.Fatal(err)
	}
	ref, name, err := resolveWaitImage(spec)
	if err != nil {
		t.Fatal(err)
	}
	if name != "boot-ok" || ref == nil || ref.W != 4 || ref.H != 4 {
		t.Fatalf("name=%s ref=%+v", name, ref)
	}
}
