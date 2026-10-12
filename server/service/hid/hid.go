package hid

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Hid struct {
	g0         *os.File
	g1         *os.File
	g2         *os.File
	kbMutex    sync.Mutex
	mouseMutex sync.Mutex
}

const (
	HID0 = "/dev/hidg0" // Keyboard
	HID1 = "/dev/hidg1" // Mouse (Relative Mode)
	HID2 = "/dev/hidg2" // Touchpad (Absolute Mode)

	hidWriteTimeout  = 80 * time.Millisecond
	hidWriteAttempts = 8
	hidWriteRetryGap = 5 * time.Millisecond
)

var (
	hid     *Hid
	hidOnce sync.Once
)

func GetHid() *Hid {
	hidOnce.Do(func() {
		hid = &Hid{}
	})
	return hid
}

func (h *Hid) Lock() {
	h.kbMutex.Lock()
	h.mouseMutex.Lock()
}

func (h *Hid) Unlock() {
	h.kbMutex.Unlock()
	h.mouseMutex.Unlock()
}

func (h *Hid) OpenNoLock() {
	var err error
	h.CloseNoLock()

	h.g0, err = os.OpenFile(HID0, os.O_WRONLY, 0o666)
	if err != nil {
		log.Errorf("open %s failed: %s", HID0, err)
	}

	h.g1, err = os.OpenFile(HID1, os.O_WRONLY, 0o666)
	if err != nil {
		log.Errorf("open %s failed: %s", HID1, err)
	}

	h.g2, err = os.OpenFile(HID2, os.O_WRONLY, 0o666)
	if err != nil {
		log.Errorf("open %s failed: %s", HID2, err)
	}
}

func (h *Hid) CloseNoLock() {
	for _, file := range []*os.File{h.g0, h.g1, h.g2} {
		if file != nil {
			_ = file.Sync()
			_ = file.Close()
		}
	}
}

func (h *Hid) Open() {
	h.kbMutex.Lock()
	defer h.kbMutex.Unlock()
	h.mouseMutex.Lock()
	defer h.mouseMutex.Unlock()

	h.CloseNoLock()

	h.OpenNoLock()
}

func (h *Hid) EnsureKeyboard() error {
	h.kbMutex.Lock()
	h.mouseMutex.Lock()
	defer h.mouseMutex.Unlock()
	defer h.kbMutex.Unlock()

	if h.g0 == nil {
		h.OpenNoLock()
	}
	if h.g0 == nil {
		return errors.New("HID keyboard is not available")
	}
	return nil
}

func (h *Hid) EnsureMouse() error {
	h.kbMutex.Lock()
	h.mouseMutex.Lock()
	defer h.mouseMutex.Unlock()
	defer h.kbMutex.Unlock()

	if h.g2 == nil {
		h.OpenNoLock()
	}
	if h.g2 == nil {
		return errors.New("HID mouse is not available")
	}
	return nil
}

func (h *Hid) KeyboardAvailable() bool {
	if _, err := os.Stat(HID0); err == nil {
		return true
	}
	h.kbMutex.Lock()
	defer h.kbMutex.Unlock()
	return h.g0 != nil
}

func (h *Hid) Close() {
	h.kbMutex.Lock()
	defer h.kbMutex.Unlock()
	h.mouseMutex.Lock()
	defer h.mouseMutex.Unlock()

	h.CloseNoLock()
}

func (h *Hid) WriteHid0(data []byte) {
	_ = h.WriteHid0Err(data)
}

func (h *Hid) WriteHid0Err(data []byte) error {
	return h.writeGadget(&h.g0, &h.kbMutex, HID0, data)
}

func (h *Hid) WriteHid1(data []byte) {
	_ = h.writeGadget(&h.g1, &h.mouseMutex, HID1, data)
}

func (h *Hid) WriteHid2(data []byte) {
	_ = h.writeGadget(&h.g2, &h.mouseMutex, HID2, data)
}

func (h *Hid) writeGadget(file **os.File, mu *sync.Mutex, path string, data []byte) error {
	var last error
	for attempt := 0; attempt < hidWriteAttempts; attempt++ {
		mu.Lock()
		if *file == nil {
			mu.Unlock()
			err := errors.New("not open")
			log.Errorf("write to %s failed: %s", path, err)
			return err
		}
		_ = (*file).SetWriteDeadline(time.Now().Add(hidWriteTimeout))
		n, err := (*file).Write(data)
		mu.Unlock()
		if err == nil && n == len(data) {
			log.Debugf("write to %s: %v", path, data)
			return nil
		}
		if err == nil {
			err = io.ErrShortWrite
		}
		last = err
		switch {
		case errors.Is(err, os.ErrClosed):
			log.Errorf("hid already closed, reopen it...")
			h.OpenNoLock()
		case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, io.ErrShortWrite):
			time.Sleep(hidWriteRetryGap)
		default:
			log.Errorf("write to %s failed: %s", path, err)
			return err
		}
	}
	if last != nil {
		log.Errorf("write to %s failed after %d attempts: %s", path, hidWriteAttempts, last)
	}
	return last
}
