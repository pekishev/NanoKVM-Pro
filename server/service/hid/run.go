package hid

import (
	"context"
	"errors"
	"strings"
	"sync"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var (
	playMu     sync.Mutex
	playCancel context.CancelFunc
)

func (s *Service) RunMacro(c *gin.Context) {
	var req proto.RunMacroReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		req.ID = ""
		req.Name = ""
	}

	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	if id == "" {
		id = strings.TrimSpace(c.Query("id"))
	}
	if name == "" {
		name = strings.TrimSpace(c.Query("name"))
	}
	if id == "" && name == "" {
		rsp.ErrRsp(c, -1, "id or name is required")
		return
	}

	macro, err := runSavedMacro(c.Request.Context(), id, name)
	if err != nil {
		switch {
		case errors.Is(err, ErrBusy):
			rsp.ErrRsp(c, -3, "HID is busy")
		case errors.Is(err, errMacroNotFound):
			rsp.ErrRsp(c, -2, "macro not found")
		case errors.Is(err, errMacroStopped), errors.Is(err, context.Canceled):
			rsp.ErrRsp(c, -4, "macro stopped")
		default:
			log.Errorf("run macro failed: %v", err)
			rsp.ErrRsp(c, -5, err.Error())
		}
		return
	}

	rsp.OkRspWithData(c, &proto.RunMacroRsp{ID: macro.ID, Name: macro.Name})
	log.Debugf("run macro %s (%s)", macro.Name, macro.ID)
}

func (s *Service) StopMacro(c *gin.Context) {
	var rsp proto.Response
	rsp.OkRspWithData(c, &proto.StopMacroRsp{Stopped: stopRunningMacro()})
}

func runSavedMacro(parent context.Context, id, name string) (proto.Macro, error) {
	if !TryLockHID() {
		return proto.Macro{}, ErrBusy
	}
	defer UnlockHID()

	macro, macros, err := lookupMacro(id, name)
	if err != nil {
		return proto.Macro{}, err
	}

	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	setPlayCancel(cancel)
	defer setPlayCancel(nil)
	defer cancel()

	if err := playMacro(ctx, macro, macros); err != nil {
		return proto.Macro{}, err
	}
	return macro, nil
}

func StopPlayback() bool {
	return stopRunningMacro()
}

func stopRunningMacro() bool {
	playMu.Lock()
	cancel := playCancel
	playMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func setPlayCancel(cancel context.CancelFunc) {
	playMu.Lock()
	playCancel = cancel
	playMu.Unlock()
}

func lookupMacro(id, name string) (proto.Macro, []proto.Macro, error) {
	macros, err := listMacros()
	if err != nil {
		return proto.Macro{}, nil, err
	}

	if id != "" {
		for _, macro := range macros {
			if macro.ID == id {
				return macro, macros, nil
			}
		}
		return proto.Macro{}, macros, errMacroNotFound
	}

	child, ok := findNamedMacro(macros, name)
	if !ok {
		return proto.Macro{}, macros, errMacroNotFound
	}
	return child, macros, nil
}
