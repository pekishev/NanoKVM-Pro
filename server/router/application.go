package router

import (
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/service/application"

	"github.com/gin-gonic/gin"
)

func applicationRouter(r *gin.Engine) {
	service := application.NewService()
	api := r.Group("/api").Use(middleware.CheckToken())

	api.GET("/application/version", service.GetVersion) // get application version
	api.POST("/application/update", service.Update)     // update from the official server or a fork release
	api.POST("/application/upload", service.Upload)     // install an uploaded release archive

	api.GET("/application/preview", service.GetPreview)  // get preview updates state
	api.POST("/application/preview", service.SetPreview) // set preview updates state

	api.GET("/application/check", service.GetUpdateCheck)  // get update check state
	api.POST("/application/check", service.SetUpdateCheck) // set update check state
}
