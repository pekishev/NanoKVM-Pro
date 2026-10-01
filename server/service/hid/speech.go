package hid

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Local speech recognizer (sherpa-onnx service on the device, see support/asr).
const speechServiceURL = "http://127.0.0.1:8766/asr"

// 60 s of 16 kHz 16-bit mono audio plus the WAV header.
const maxSpeechBytes = 60*16000*2 + 1024

var speechClient = &http.Client{Timeout: 90 * time.Second}

type speechRsp struct {
	Text string `json:"text"`
}

func (s *Service) Speech(c *gin.Context) {
	var rsp proto.Response

	wav, err := io.ReadAll(io.LimitReader(c.Request.Body, maxSpeechBytes+1))
	if err != nil || len(wav) < 44 {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}
	if len(wav) > maxSpeechBytes {
		rsp.ErrRsp(c, -1, "audio too long")
		return
	}

	res, err := speechClient.Post(speechServiceURL, "audio/wav", bytes.NewReader(wav))
	if err != nil {
		log.Errorf("speech service request failed: %v", err)
		rsp.ErrRsp(c, -2, "speech service unavailable")
		return
	}
	defer res.Body.Close()

	var out speechRsp
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&out) != nil {
		log.Errorf("speech service failed with status %d", res.StatusCode)
		rsp.ErrRsp(c, -3, "speech recognition failed")
		return
	}

	rsp.OkRspWithData(c, &out)
	log.Debugf("speech recognized: %d bytes of audio, %d characters", len(wav), len(out.Text))
}
