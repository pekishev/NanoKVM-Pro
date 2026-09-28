package hid

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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
)

var (
	errInvalidMacroImage  = errors.New("invalid macro image")
	macroIDPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	macroImageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)
)

type macroImageMeta struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type storedMacroImage struct {
	X   float64
	Y   float64
	W   float64
	H   float64
	Png []byte
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

	if err := writeMacroImage(req.ID, req.Name, req.X, req.Y, req.W, req.H, raw); err != nil {
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

	rsp.OkRspWithData(c, &proto.MacroImageRsp{
		Name: req.Name,
		X:    image.X,
		Y:    image.Y,
		W:    image.W,
		H:    image.H,
		Png:  base64.StdEncoding.EncodeToString(image.Png),
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

func writeMacroImage(id, name string, x, y, w, h float64, raw []byte) error {
	if err := validateMacroImageRef(id, name); err != nil {
		return err
	}
	if err := validateMacroImageRect(x, y, w, h); err != nil {
		return err
	}

	encoded, err := normalizeMacroPNG(raw)
	if err != nil {
		return err
	}

	dir := filepath.Join(macroImageRoot, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	meta, err := json.Marshal(macroImageMeta{X: x, Y: y, W: w, H: h})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), meta, 0o644); err != nil {
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

	return &storedMacroImage{
		X:   meta.X,
		Y:   meta.Y,
		W:   meta.W,
		H:   meta.H,
		Png: pngBytes,
	}, nil
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

func validateMacroImageRect(x, y, w, h float64) error {
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

func normalizeMacroPNG(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxMacroImageBytes {
		return nil, errInvalidMacroImage
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errInvalidMacroImage
	}

	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 || bounds.Dx() > maxMacroImageEdge || bounds.Dy() > maxMacroImageEdge {
		return nil, errInvalidMacroImage
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
