package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxBody = 2 << 20

func Handle(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("MCP-Protocol-Version", protocolFallback)
	switch c.Request.Method {
	case http.MethodGet:
		c.Header("Allow", "POST, DELETE")
		c.Status(http.StatusMethodNotAllowed)
		return
	case http.MethodDelete:
		c.Status(http.StatusNoContent)
		return
	case http.MethodPost:
	default:
		c.Header("Allow", "POST, DELETE")
		c.Status(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil {
		writeRPC(c, rpcErr(nil, -32700, "parse error"))
		return
	}
	if len(body) > maxBody {
		writeRPC(c, rpcErr(nil, -32700, "request too large"))
		return
	}
	body = bytesTrim(body)
	if len(body) == 0 {
		c.Status(http.StatusAccepted)
		return
	}

	if body[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			writeRPC(c, rpcErr(nil, -32700, "parse error"))
			return
		}
		out := make([]rpcResponse, 0, len(batch))
		for _, req := range batch {
			if resp := handleRPCCtx(c.Request.Context(), req); resp != nil {
				out = append(out, *resp)
			}
		}
		if len(out) == 0 {
			c.Status(http.StatusAccepted)
			return
		}
		c.JSON(http.StatusOK, out)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPC(c, rpcErr(nil, -32700, "parse error"))
		return
	}
	resp := handleRPCCtx(c.Request.Context(), req)
	if resp == nil {
		c.Status(http.StatusAccepted)
		return
	}
	if session := strings.TrimSpace(c.GetHeader("Mcp-Session-Id")); session != "" {
		c.Header("Mcp-Session-Id", session)
	}
	writeRPC(c, resp)
}

func writeRPC(c *gin.Context, resp *rpcResponse) {
	c.JSON(http.StatusOK, resp)
}

func bytesTrim(body []byte) []byte {
	i := 0
	j := len(body)
	for i < j && (body[i] == ' ' || body[i] == '\n' || body[i] == '\r' || body[i] == '\t') {
		i++
	}
	for j > i && (body[j-1] == ' ' || body[j-1] == '\n' || body[j-1] == '\r' || body[j-1] == '\t') {
		j--
	}
	return body[i:j]
}
