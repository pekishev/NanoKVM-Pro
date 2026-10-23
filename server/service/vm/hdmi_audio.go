package vm

import (
	"NanoKVM-Server/proto"
	"NanoKVM-Server/service/stream"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func (s *Service) GetHdmiAudio(c *gin.Context) {
	var rsp proto.Response

	rsp.OkRspWithData(c, &proto.GetHdmiAudioRsp{
		Enabled: stream.IsHdmiAudioEnabled(),
	})
	log.Debugf("get HDMI audio status: %t", stream.IsHdmiAudioEnabled())
}

func (s *Service) SetHdmiAudio(c *gin.Context) {
	var req proto.SetHdmiAudioReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	if err := stream.SetHdmiAudioEnabled(req.Enabled); err != nil {
		log.Errorf("failed to set HDMI audio: %s", err)
		rsp.ErrRsp(c, -2, "failed to set HDMI audio")
		return
	}

	rsp.OkRsp(c)
	log.Debugf("set HDMI audio status: %t", req.Enabled)
}
