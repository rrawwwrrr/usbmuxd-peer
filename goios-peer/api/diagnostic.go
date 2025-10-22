package api

import (
	"net/http"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/diagnostics"
	"github.com/gin-gonic/gin"
)

// Перезапуск устройства
// @Summary      Перезапуск устройства
// @Description  Перезапуск устройства
// @Tags         device
// @Produce      json
// @Param        udid path string true "UDID устройства"
// @Success      200 {object} GenericResponse
// @Failure      500 {object} GenericResponse
// @Router       /device/{udid}/reboot [post]
func Reboot(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	err := diagnostics.Reboot(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}
