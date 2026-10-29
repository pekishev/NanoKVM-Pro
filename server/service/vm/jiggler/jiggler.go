package jiggler

import (
	"NanoKVM-Server/service/hid"
	"os"
	"sync"
	"time"
)

const (
	Interval   = 60 * time.Second
	PulseGap   = 50 * time.Millisecond
	pulseDelta = 5
	pulseMode  = "relative"
)

var ConfigFile = "/etc/kvm/mouse-jiggler"

var (
	jiggler Jiggler
	once    sync.Once
)

type Jiggler struct {
	mutex       sync.Mutex
	enabled     bool
	running     bool
	lastUpdated time.Time
}

func GetJiggler() *Jiggler {
	once.Do(func() {
		jiggler = Jiggler{
			enabled:     false,
			running:     false,
			lastUpdated: time.Now(),
		}

		if _, err := os.Stat(ConfigFile); err != nil {
			return
		}

		jiggler.enabled = true
	})

	return &jiggler
}

func (j *Jiggler) Enable() error {
	err := os.WriteFile(ConfigFile, []byte(pulseMode), 0644)
	if err != nil {
		return err
	}

	j.mutex.Lock()
	j.enabled = true
	j.mutex.Unlock()
	j.Run()

	return nil
}

func (j *Jiggler) Disable() error {
	if err := os.Remove(ConfigFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	j.mutex.Lock()
	j.enabled = false
	j.mutex.Unlock()

	return nil
}

func (j *Jiggler) Run() {
	j.mutex.Lock()
	if !j.enabled || j.running {
		j.mutex.Unlock()
		return
	}
	j.running = true
	j.mutex.Unlock()

	j.Update()

	go func() {
		ticker := time.NewTicker(Interval)
		defer ticker.Stop()

		for range ticker.C {
			j.mutex.Lock()
			if !j.enabled {
				j.running = false
				j.mutex.Unlock()
				return
			}
			idle := time.Since(j.lastUpdated) >= Interval
			j.mutex.Unlock()

			if idle {
				move()
				j.Update()
			}
		}
	}()
}

func (j *Jiggler) Update() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	if j.running {
		j.lastUpdated = time.Now()
	}
}

func (j *Jiggler) IsEnabled() bool {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	return j.enabled
}

func (j *Jiggler) GetMode() string {
	return pulseMode
}

func relativePulseReports() (forward, back []byte) {
	return []byte{0x00, pulseDelta, 0x00, 0x00}, []byte{0x00, 256 - pulseDelta, 0x00, 0x00}
}

func move() {
	h := hid.GetHid()
	forward, back := relativePulseReports()
	h.WriteHid1(forward)
	time.Sleep(PulseGap)
	h.WriteHid1(back)
}
