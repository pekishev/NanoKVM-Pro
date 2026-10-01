package router

import (
	"github.com/gin-gonic/gin"

	"NanoKVM-Server/middleware"
	"NanoKVM-Server/service/hid"
)

func hidRouter(r *gin.Engine) {
	service := hid.NewService()
	api := r.Group("/api").Use(middleware.CheckToken())

	api.POST("/hid/reset", service.Reset)   // reset hid
	api.POST("/hid/paste", service.Paste)   // paste
	api.POST("/hid/speech", service.Speech) // local speech recognition

	api.GET("/hid/macros", service.GetMacros)          // get macros
	api.POST("/hid/macro", service.AddMacro)           // add macro
	api.POST("/hid/macro/update", service.UpdateMacro) // update macro
	api.DELETE("/hid/macro", service.DeleteMacro)      // delete macro
	api.POST("/hid/macro/image", service.SaveMacroImage)
	api.GET("/hid/macro/image", service.GetMacroImage)

	api.GET("/hid/mode", service.GetHidMode)  // get hid mode
	api.POST("/hid/mode", service.SetHidMode) // set hid mode
}
