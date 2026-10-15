package hid

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Reference fragments are stored next to the macro script:
// /etc/kvm/macros.json and /etc/kvm/macros/<id>/<name>.png (+ <name>.json with the rectangle).
var macroImageRoot = "/etc/kvm/macros"

const (
	maxMacroImageBytes = 512 * 1024
	maxMacroImageEdge  = 320
	maxMacroFrame      = 4096
)

var (
	errInvalidMacroImage  = errors.New("invalid macro image")
	macroIDPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	macroImageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)
)

type macroImageMeta struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	FrameW int     `json:"frameW,omitempty"`
	FrameH int     `json:"frameH,omitempty"`
}

type storedMacroImage struct {
	X, Y, W, H     int
	FrameW, FrameH int
	PX, PY, PW, PH float64
	Png            []byte
}

func (s *Service) SaveMacroImage(c *gin.Context) {
	var req proto.SaveMacroImageReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	raw, err := base64.StdEncoding.DecodeString(req.Png)
	if err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	macroMutex.Lock()
	defer macroMutex.Unlock()

	store, err := loadMacros()
	if err != nil {
		log.Errorf("failed to save macro image: %v", err)
		rsp.ErrRsp(c, -2, "save image failed")
		return
	}
	if !macroExists(store, req.ID) {
		rsp.ErrRsp(c, -2, "macro not found")
		return
	}

	meta := macroImageMeta{X: req.X, Y: req.Y, W: req.W, H: req.H, FrameW: req.FrameW, FrameH: req.FrameH}
	if err := writeMacroImage(req.ID, req.Name, meta, raw); err != nil {
		if errors.Is(err, errInvalidMacroImage) {
			rsp.ErrRsp(c, -1, "invalid arguments")
			return
		}
		log.Errorf("failed to save macro image: %v", err)
		rsp.ErrRsp(c, -2, "save image failed")
		return
	}

	rsp.OkRsp(c)
	log.Debugf("save macro image %s/%s", req.ID, req.Name)
}

func (s *Service) CaptureMacroImage(c *gin.Context) {
	var req proto.CaptureMacroImageReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	frame, err := grabFrameFn()
	if err != nil || frame == nil {
		rsp.ErrRsp(c, -2, "HDMI frame unavailable")
		return
	}

	srcW, srcH := req.FrameW, req.FrameH
	if srcW < 1 || srcH < 1 {
		srcW, srcH = frame.Width, frame.Height
	}
	region, ok := mapPixelRect(req.X, req.Y, req.W, req.H, srcW, srcH, frame.Width, frame.Height)
	if !ok {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	pix := copyCrop(frame, region.x, region.y, region.w, region.h)
	if pix == nil {
		rsp.ErrRsp(c, -2, "HDMI frame unavailable")
		return
	}
	encoded, err := pngFromRGBA(region.w, region.h, pix)
	if err != nil {
		rsp.ErrRsp(c, -2, "capture image failed")
		return
	}

	rsp.OkRspWithData(c, &proto.MacroImageRsp{
		Name:   "",
		X:      float64(region.x),
		Y:      float64(region.y),
		W:      float64(region.w),
		H:      float64(region.h),
		FrameW: frame.Width,
		FrameH: frame.Height,
		Png:    base64.StdEncoding.EncodeToString(encoded),
	})
}

func (s *Service) GetMacroImage(c *gin.Context) {
	var req proto.GetMacroImageReq
	var rsp proto.Response

	if err := proto.ParseQueryRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	macroMutex.RLock()
	image, err := readMacroImage(req.ID, req.Name)
	macroMutex.RUnlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errInvalidMacroImage) {
			rsp.ErrRsp(c, -2, "image not found")
			return
		}
		log.Errorf("failed to read macro image: %v", err)
		rsp.ErrRsp(c, -2, "read image failed")
		return
	}

	x, y, w, h := float64(image.X), float64(image.Y), float64(image.W), float64(image.H)
	if image.FrameW < 1 {
		x, y, w, h = image.PX, image.PY, image.PW, image.PH
	}
	rsp.OkRspWithData(c, &proto.MacroImageRsp{
		Name:   req.Name,
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
		FrameW: image.FrameW,
		FrameH: image.FrameH,
		Png:    base64.StdEncoding.EncodeToString(image.Png),
	})
}

func macroExists(store *MacroStore, id string) bool {
	for _, macro := range store.Macros {
		if macro.ID == id {
			return true
		}
	}
	return false
}

func writeMacroImage(id, name string, meta macroImageMeta, raw []byte) error {
	if err := validateMacroImageRef(id, name); err != nil {
		return err
	}

	encoded, width, height, err := normalizeMacroPNG(raw)
	if err != nil {
		return err
	}
	if err := validateMacroImageMeta(meta, width, height); err != nil {
		return err
	}

	dir := filepath.Join(macroImageRoot, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), body, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".png"), encoded, 0o644)
}

func readMacroImage(id, name string) (*storedMacroImage, error) {
	if err := validateMacroImageRef(id, name); err != nil {
		return nil, err
	}

	dir := filepath.Join(macroImageRoot, id)
	metaBytes, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	pngBytes, err := os.ReadFile(filepath.Join(dir, name+".png"))
	if err != nil {
		return nil, err
	}

	var meta macroImageMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}

	stored := &storedMacroImage{Png: pngBytes}
	if meta.FrameW > 0 && meta.FrameH > 0 {
		stored.X = int(math.Round(meta.X))
		stored.Y = int(math.Round(meta.Y))
		stored.W = int(math.Round(meta.W))
		stored.H = int(math.Round(meta.H))
		stored.FrameW = meta.FrameW
		stored.FrameH = meta.FrameH
	} else {
		stored.PX, stored.PY, stored.PW, stored.PH = meta.X, meta.Y, meta.W, meta.H
	}
	return stored, nil
}

func removeMacroImages(id string) error {
	if !macroIDPattern.MatchString(id) {
		return errInvalidMacroImage
	}
	return os.RemoveAll(filepath.Join(macroImageRoot, id))
}

func validateMacroImageRef(id, name string) error {
	if !macroIDPattern.MatchString(id) || !macroImageNamePattern.MatchString(name) {
		return errInvalidMacroImage
	}
	return nil
}

func validateMacroImageMeta(meta macroImageMeta, pngW, pngH int) error {
	if meta.FrameW > 0 && meta.FrameH > 0 {
		x, y := int(math.Round(meta.X)), int(math.Round(meta.Y))
		w, h := int(math.Round(meta.W)), int(math.Round(meta.H))
		if float64(x) != meta.X || float64(y) != meta.Y || float64(w) != meta.W || float64(h) != meta.H {
			return errInvalidMacroImage
		}
		if w != pngW || h != pngH {
			return errInvalidMacroImage
		}
		if meta.FrameW > maxMacroFrame || meta.FrameH > maxMacroFrame {
			return errInvalidMacroImage
		}
		_, ok := clampPixelRect(x, y, w, h, meta.FrameW, meta.FrameH)
		if !ok {
			return errInvalidMacroImage
		}
		return nil
	}
	return validatePercentRect(meta.X, meta.Y, meta.W, meta.H)
}

func validatePercentRect(x, y, w, h float64) error {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(w) || math.IsNaN(h) {
		return errInvalidMacroImage
	}
	if math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return errInvalidMacroImage
	}
	if x < 0 || y < 0 || x > 100 || y > 100 || w <= 0 || h <= 0 || w > 100 || h > 100 {
		return errInvalidMacroImage
	}
	if x+w > 100.01 || y+h > 100.01 {
		return errInvalidMacroImage
	}
	return nil
}

func normalizeMacroPNG(raw []byte) ([]byte, int, int, error) {
	if len(raw) == 0 || len(raw) > maxMacroImageBytes {
		return nil, 0, 0, errInvalidMacroImage
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, errInvalidMacroImage
	}

	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 || bounds.Dx() > maxMacroImageEdge || bounds.Dy() > maxMacroImageEdge {
		return nil, 0, 0, errInvalidMacroImage
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), bounds.Dx(), bounds.Dy(), nil
}

func pngFromRGBA(width, height int, pix []byte) ([]byte, error) {
	if width < 1 || height < 1 || len(pix) != width*height*4 {
		return nil, errInvalidMacroImage
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, pix)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
