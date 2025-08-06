package rest

import (
	"github.com/danielpaulus/go-ios/restapi/api"
	"github.com/gin-gonic/gin"
)

var streamingMiddleWare = api.StreamingHeaderMiddleware()

func registerRoutes(router *gin.RouterGroup) {
	router.GET("/list", api.List)

	device := router.Group("/device/:udid")
	device.Use(api.DeviceMiddleware())
	simpleDeviceRoutes(device)
	appRoutes(device)
}

func simpleDeviceRoutes(device *gin.RouterGroup) {
	device.POST("/activate", api.Activate)

	device.GET("/conditions", api.GetSupportedConditions)
	device.PUT("/enable-condition", api.EnableDeviceCondition)
	device.POST("/disable-condition", api.DisableDeviceCondition)

	device.GET("/image", api.GetImages)
	device.PUT("/image", api.InstallImage)

	device.GET("/notifications", streamingMiddleWare, api.Notifications)

	device.GET("/info", api.Info)
	device.GET("/listen", streamingMiddleWare, api.Listen)

	device.POST("/pair", api.PairDevice)
	device.GET("/profiles", api.GetProfiles)

	device.POST("/resetlocation", api.ResetLocation)
	device.GET("/screenshot", api.Screenshot)
	device.PUT("/setlocation", api.SetLocation)
	device.GET("/syslog", streamingMiddleWare, api.Syslog)

	device.POST("/wda/session", api.CreateWdaSession)
	device.GET("/wda/session/:sessionId", api.ReadWdaSession)
	device.DELETE("/wda/session/:sessionId", api.DeleteWdaSession)
}

func appRoutes(group *gin.RouterGroup) {
	router := group.Group("/apps")
	router.Use(api.LimitNumClientsUDID())
	router.GET("/", api.ListApps)
	router.POST("/launch", api.LaunchApp)
	router.POST("/kill", api.KillApp)
	router.POST("/install", api.InstallApp)
	router.POST("/uninstall", api.UninstallApp)
}
