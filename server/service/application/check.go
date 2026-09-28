package application

import (
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/proto"
)

const updateCheckDisabledFlag = "/etc/kvm/update_check_disabled"

func (s *Service) GetUpdateCheck(c *gin.Context) {
	var rsp proto.Response

	enabled := isUpdateCheckEnabled()
	rsp.OkRspWithData(c, &proto.GetUpdateCheckRsp{Enabled: enabled})
	log.Debugf("get update check state: %t", enabled)
}

func (s *Service) SetUpdateCheck(c *gin.Context) {
	var req proto.SetUpdateCheckReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	if err := setUpdateCheckEnabled(req.Enable); err != nil {
		log.Errorf("failed to set update check state: %s", err)
		rsp.ErrRsp(c, -2, "failed to save update check state")
		return
	}

	rsp.OkRsp(c)
	log.Debugf("set update check state: %t", req.Enable)
}

func isUpdateCheckEnabled() bool {
	_, err := os.Stat(updateCheckDisabledFlag)
	return err != nil
}

func setUpdateCheckEnabled(enable bool) error {
	if enable {
		err := os.Remove(updateCheckDisabledFlag)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.MkdirAll(filepath.Dir(updateCheckDisabledFlag), 0o755); err != nil {
		return err
	}

	return os.WriteFile(updateCheckDisabledFlag, []byte("1\n"), 0o644)
}
