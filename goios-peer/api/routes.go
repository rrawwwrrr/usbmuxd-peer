package api

import (
	"github.com/gin-gonic/gin"
)

var streamingMiddleWare = StreamingHeaderMiddleware()
var mjpegMiddleWare = MJPEGHeadersMiddleware()

func registerRoutes(router *gin.RouterGroup) {
	device := router.Group("/device/:udid")
	device.Use(DeviceMiddleware())
	simpleDeviceRoutes(device)
	appRoutes(device)
	wdaRoutes(device)
	streamRoutes(device)
	conditionsRoutes(device)
}

func websocketRoutes(router *gin.Engine) {
	router.GET("/ws", func(c *gin.Context) {
		serveWs(c.Writer, c.Request)
	})
}
func downLoadRoutes(router *gin.Engine) {
	router.GET("/ddi-15F31d.zip", Download)
}

func simpleDeviceRoutes(device *gin.RouterGroup) {
	device.POST("/activate", Activate)

	device.GET("/image", GetImages)
	device.POST("/image", InstallImage)

	device.GET("/notifications", streamingMiddleWare, Notifications)

	device.GET("/info", Info)
	device.GET("/listen", streamingMiddleWare, Listen)

	device.POST("/pair", PairDevice)
	device.GET("/profiles", GetProfiles)

	device.POST("/resetlocation", ResetLocation)
	device.GET("/screenshot", Screenshot)
	device.GET("/screenstream", mjpegMiddleWare, MJPEGStreamHandler)
	device.PUT("/setlocation", SetLocation)
	device.GET("/syslog", streamingMiddleWare, Syslog)

}

func conditionsRoutes(group *gin.RouterGroup) {
	router := group.Group("/conditions")
	router.GET("/", GetSupportedConditions)
	router.PUT("/enable", EnableDeviceCondition)
	router.POST("/disable", DisableDeviceCondition)
}

func appRoutes(group *gin.RouterGroup) {
	router := group.Group("/apps")
	router.Use(LimitNumClientsUDID())
	router.GET("/", ListApps)
	router.POST("/launch", LaunchApp)
	router.POST("/kill", KillApp)
	router.POST("/install", InstallApp)
	router.DELETE("/uninstall", UninstallApp)
}

func wdaRoutes(group *gin.RouterGroup) {
	router := group.Group("/wda")
	router.POST("/session", CreateWdaSession)
	router.GET("/stream", mjpegMiddleWare, MJPEGProxyHandler)
	router.GET("/session", ReadWdaSession)
	router.DELETE("/session", DeleteWdaSession)
}

func streamRoutes(group *gin.RouterGroup) {
	router := group.Group("/stream")
	router.Use(LimitNumClientsUDID())
	router.POST("/start", StartStream)
	router.DELETE("/stop", StopStream)
	router.GET("/status", StatusStream)
}
