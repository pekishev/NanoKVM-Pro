package hid

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image/jpeg"
	"image/png"
	"strings"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const waitImageDefaultMS = 20000

var (
	ErrWaitImageNotFound = errors.New("image not found")
	ErrWaitImageNoFrame  = errors.New("HDMI frame unavailable")
	ErrWaitImageTimeout  = errors.New("WAITIMAGE timed out")
	ErrStopped           = errMacroStopped
	errWaitImageArgs     = errors.New("invalid arguments")
)

func (s *Service) WaitImage(c *gin.Context) {
	var req proto.WaitImageReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	spec, err := WaitImageSpecFromReq(req)
	if err != nil {
		rsp.ErrRsp(c, -1, err.Error())
		return
	}

	name, err := runWaitImage(c.Request.Context(), spec)
	if err != nil {
		switch {
		case errors.Is(err, ErrBusy):
			rsp.ErrRsp(c, -3, "HID is busy")
		case errors.Is(err, errWaitImageArgs):
			rsp.ErrRsp(c, -1, err.Error())
		case errors.Is(err, errMacroNotFound), errors.Is(err, ErrWaitImageNotFound):
			rsp.ErrRsp(c, -2, "image not found")
		case errors.Is(err, errMacroStopped), errors.Is(err, context.Canceled):
			rsp.ErrRsp(c, -4, "stopped")
		case errors.Is(err, ErrWaitImageNoFrame):
			rsp.ErrRsp(c, -5, "HDMI frame unavailable")
		case errors.Is(err, ErrWaitImageTimeout):
			if spec.Name != "" {
				rsp.ErrRsp(c, -5, "WAITIMAGE "+spec.Name+" timed out")
				return
			}
			rsp.ErrRsp(c, -5, ErrWaitImageTimeout.Error())
		default:
			log.Errorf("wait image failed: %v", err)
			rsp.ErrRsp(c, -5, err.Error())
		}
		return
	}

	rsp.OkRspWithData(c, &proto.WaitImageRsp{Matched: true, Name: name})
	log.Debugf("wait image matched %s", name)
}

func runWaitImage(parent context.Context, spec WaitImageSpec) (string, error) {
	if !TryLockHID() {
		return "", ErrBusy
	}
	defer UnlockHID()
	return PlayWaitImage(parent, spec)
}

func PlayWaitImage(parent context.Context, spec WaitImageSpec) (string, error) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	setPlayCancel(cancel)
	defer setPlayCancel(nil)
	defer cancel()

	ref, name, err := resolveWaitImage(spec)
	if err != nil {
		return "", err
	}
	if err := waitForLoadedImage(ctx, ref, spec.Ms); err != nil {
		return "", err
	}
	return name, nil
}

type WaitImageSpec struct {
	ID     string
	Macro  string
	Name   string
	Ms     int
	X      int
	Y      int
	W      int
	H      int
	FrameW int
	FrameH int
	Png    string
}

func WaitImageSpecFromReq(req proto.WaitImageReq) (WaitImageSpec, error) {
	spec := WaitImageSpec{
		ID:     strings.TrimSpace(req.ID),
		Macro:  strings.TrimSpace(req.Macro),
		Name:   strings.TrimSpace(req.Name),
		Ms:     waitImageDefaultMS,
		X:      req.X,
		Y:      req.Y,
		W:      req.W,
		H:      req.H,
		FrameW: req.FrameW,
		FrameH: req.FrameH,
		Png:    strings.TrimSpace(req.Png),
	}
	if req.Ms != nil {
		spec.Ms = *req.Ms
	}
	if spec.Ms < 0 || spec.Ms > macroImageTimeoutMax {
		return WaitImageSpec{}, errWaitImageArgs
	}
	if spec.Png == "" && spec.Name == "" {
		return WaitImageSpec{}, errWaitImageArgs
	}
	if spec.Name != "" && !macroImageNameRe.MatchString(spec.Name) {
		return WaitImageSpec{}, errWaitImageArgs
	}
	return spec, nil
}

func resolveWaitImage(spec WaitImageSpec) (*loadedMacroImage, string, error) {
	if spec.Png != "" {
		ref, err := loadedImageFromInline(spec)
		if err != nil {
			if errors.Is(err, errInvalidMacroImage) {
				return nil, "", errWaitImageArgs
			}
			return nil, "", err
		}
		name := spec.Name
		if name == "" {
			name = "inline"
		}
		return ref, name, nil
	}

	id := spec.ID
	if id == "" && spec.Macro != "" {
		macro, _, err := lookupMacro("", spec.Macro)
		if err != nil {
			if errors.Is(err, errMacroNotFound) {
				return nil, "", ErrWaitImageNotFound
			}
			return nil, "", err
		}
		id = macro.ID
	}
	if id == "" {
		found, err := findMacroIDForImage(spec.Name)
		if err != nil {
			return nil, "", err
		}
		id = found
	}

	ref, err := loadImageFn(id, spec.Name)
	if err != nil {
		return nil, "", ErrWaitImageNotFound
	}
	return ref, spec.Name, nil
}

func loadedImageFromInline(spec WaitImageSpec) (*loadedMacroImage, error) {
	raw, err := decodeBase64Image(spec.Png)
	if err != nil {
		return nil, err
	}

	width, height, pix, err := decodeRefPixels(raw)
	if err != nil {
		return nil, err
	}

	frameW, frameH := spec.FrameW, spec.FrameH
	if frameW < 1 || frameH < 1 {
		var sizeErr error
		frameW, frameH, sizeErr = frameSizeFn()
		if sizeErr != nil || frameW < 1 || frameH < 1 {
			return nil, ErrWaitImageNoFrame
		}
	}

	x, y, w, h := spec.X, spec.Y, spec.W, spec.H
	if w < 1 || h < 1 {
		w, h = width, height
	}
	meta := macroImageMeta{
		X: float64(x), Y: float64(y), W: float64(w), H: float64(h),
		FrameW: frameW, FrameH: frameH,
	}
	if err := validateMacroImageMeta(meta, width, height); err != nil {
		return nil, err
	}

	return &loadedMacroImage{
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
		FrameW: frameW,
		FrameH: frameH,
		Width:  width,
		Height: height,
		Data:   pix,
	}, nil
}

func findMacroIDForImage(name string) (string, error) {
	macros, err := listMacros()
	if err != nil {
		return "", err
	}
	for _, macro := range macros {
		if macro.ID == "" {
			continue
		}
		if _, err := readMacroImage(macro.ID, name); err == nil {
			return macro.ID, nil
		}
	}
	return "", ErrWaitImageNotFound
}

func decodeBase64Image(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(strings.ToLower(raw[:i]), "base64") {
		raw = raw[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return nil, errInvalidMacroImage
	}
	if len(data) == 0 || len(data) > maxMacroImageBytes {
		return nil, errInvalidMacroImage
	}
	return data, nil
}

func decodeRefPixels(raw []byte) (int, int, []byte, error) {
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		img, err = jpeg.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return 0, 0, nil, errInvalidMacroImage
	}

	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 || bounds.Dx() > maxMacroImageEdge || bounds.Dy() > maxMacroImageEdge {
		return 0, 0, nil, errInvalidMacroImage
	}
	width, height, pix := imagePixels(img)
	if width < 1 || height < 1 || pix == nil {
		return 0, 0, nil, errInvalidMacroImage
	}
	return width, height, pix, nil
}
