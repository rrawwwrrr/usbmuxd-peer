package api

import (
	"io"
	"net/http"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/syslog"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Уведомления используют instruments для получения событий изменения состояния приложений.
// События будут передаваться как JSON-объекты, разделённые переносами строк, до возникновения ошибки.
// Listen                godoc
// @Summary      Использует instruments для получения событий изменения состояния приложений
// @Description Использует instruments для получения событий изменения состояния приложений
// @Tags         general
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /notifications [get]
func Notifications(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	listenerFunc, closeFunc, err := instruments.ListenAppStateNotifications(device)
	if err != nil {
		log.Fatal(err)
	}
	c.Stream(func(w io.Writer) bool {

		notification, err := listenerFunc()
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			closeFunc()
			return false
		}

		_, err = w.Write([]byte(MustMarshal(notification)))

		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			closeFunc()
			return false
		}
		w.Write([]byte("\n"))
		return true
	})

}

// Syslog
// Listen                godoc
// @Summary      Использует SSE для подключения к команде LISTEN
// @Description Использует SSE для подключения к команде LISTEN
// @Tags         general
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router      /device/{udid}/stream/listen [get]
func Syslog(c *gin.Context) {
	// We are streaming current time to clients in the interval 10 seconds
	log.Info("connect")
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	syslogConnection, err := syslog.New(device)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}
	c.Stream(func(w io.Writer) bool {
		m, _ := syslogConnection.ReadLogMessage()
		// Stream message to client from message channel
		w.Write([]byte(MustMarshal(m)))
		return true
	})
}

// Listen отправляет события с сервера (SSE), когда устройства подключаются или отключаются
// Listen                godoc
// @Summary      Использует SSE для подключения к команде LISTEN
// @Description Использует SSE для подключения к команде LISTEN
// @Tags         general
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /device/{udid}/stream/listen [get]
func Listen(c *gin.Context) {
	// We are streaming current time to clients in the interval 10 seconds
	log.Info("connect")
	a, _, _ := ios.Listen()
	c.Stream(func(w io.Writer) bool {
		l, _ := a()
		// Stream message to client from message channel
		w.Write([]byte(MustMarshal(l)))
		return true
	})
}
