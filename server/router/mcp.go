package router

import (
	"NanoKVM-Server/service/mcp"

	"github.com/gin-gonic/gin"
)

func mcpRouter(r *gin.Engine) {
	group := r.Group("/mcp").Use(mcp.CheckAuth())
	group.Any("", mcp.Handle)
	group.Any("/", mcp.Handle)
}
