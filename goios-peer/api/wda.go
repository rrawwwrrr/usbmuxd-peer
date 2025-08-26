package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/testmanagerd"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type WdaConfig struct {
	BundleID     string                 `json:"bundleId" binding:"required"`
	TestbundleID string                 `json:"testBundleId" binding:"required"`
	XCTestConfig string                 `json:"xcTestConfig" binding:"required"`
	Args         []string               `json:"args"`
	Env          map[string]interface{} `json:"env"`
}

type WdaSessionKey struct {
	udid      string
	sessionID string
}

type WdaSession struct {
	Config    WdaConfig `json:"config" binding:"required"`
	SessionId string    `json:"sessionId" binding:"required"`
	Udid      string    `json:"udid" binding:"required"`
	stopWda   context.CancelFunc
}

func (session *WdaSession) Write(p []byte) (n int, err error) {
	log.
		WithField("udid", session.Udid).
		WithField("sessionId", session.SessionId).
		Debugf("WDA_LOG %s", p)

	return len(p), nil
}

var globalSessions = sync.Map{}

// @Summary Создать новую сессию WDA
// @Description Создать новую сессию WebDriverAgent для указанного устройства
// @Tags WebDriverAgent
// @Accept json
// @Produce json
// @Param config body WdaConfig true "Конфигурация WebDriverAgent"
// @Success 200 {object} WdaSession
// @Failure 400 {object} GenericResponse
// @Router /wda/session [post]
func CreateWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	_, existingSession, found := FindSessionByUdid(device.Properties.SerialNumber)
	if found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session already exists for this device", "session": existingSession})
		return
	}

	log.
		WithField("udid", device.Properties.SerialNumber).
		Debugf("Creating WDA session")

	var config WdaConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		config = WdaConfig{
			BundleID:     "com.facebook.WebDriverAgentRunner.xctrunner",
			TestbundleID: "com.facebook.WebDriverAgentRunner.xctrunner",
			XCTestConfig: "WebDriverAgentRunner.xctest",
			Args:         []string{},
			Env: map[string]interface{}{
				"MJPEG_SERVER_PORT":         "8001",
				"USE_PORT":                  "8100",
				"UITEST_DISABLE_ANIMATIONS": "YES",
			},
		}
	}
	sessionKey := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: uuid.New().String(),
	}

	wdaCtx, stopWda := context.WithCancel(context.Background())

	session := WdaSession{
		Udid:      sessionKey.udid,
		SessionId: sessionKey.sessionID,
		Config:    config,
		stopWda:   stopWda,
	}
	go func() {
		/* прокидываем порт для mjpeg трафика*/
		mjpegPortStr, ok := config.Env["MJPEG_SERVER_PORT"].(string)
		if !ok {
			log.Error("MJPEG_SERVER_PORT is not a string")
			return
		}
		mjpegPort, err := strconv.ParseUint(mjpegPortStr, 10, 16)
		if err != nil {
			log.Errorf("Invalid MJPEG_SERVER_PORT: %v", err)
			return
		}
		fwdMjpeg, err := forward.Forward(device, uint16(mjpegPort), uint16(mjpegPort))
		if err != nil {
			log.Info(err)
		}
		log.
			WithField("udid", device.Properties.SerialNumber).
			WithField("port", mjpegPort).
			Debugf("PortForward mjpeg server")

		/* прокидываем порт для wda трафика */
		usePortStr, ok := config.Env["USE_PORT"].(string)
		if !ok {
			log.Error("USE_PORT is not a string")
			return
		}
		usePort, err := strconv.ParseUint(usePortStr, 10, 16)
		if err != nil {
			log.Errorf("Invalid USE_PORT: %v", err)
			return
		}
		fwdWda, err := forward.Forward(device, uint16(usePort), uint16(usePort))
		if err != nil {
			log.Info(err)
		}
		log.
			WithField("udid", device.Properties.SerialNumber).
			WithField("port", fwdWda).
			Debugf("PortForward wda server")

		/* запускаем wda */
		_, err = testmanagerd.RunTestWithConfig(wdaCtx, testmanagerd.TestConfig{
			BundleId:           config.BundleID,
			TestRunnerBundleId: config.TestbundleID,
			XctestConfigName:   config.XCTestConfig,
			Env:                config.Env,
			Args:               config.Args,
			Device:             device,
			Listener:           testmanagerd.NewTestListener(&session, &session, os.TempDir()),
		})
		if err != nil {
			log.
				WithField("udid", sessionKey.udid).
				WithField("sessionId", sessionKey.sessionID).
				WithError(err).
				Error("Failed running WDA")
		}

		stopWda()
		fwdMjpeg.Close()
		fwdWda.Close()
		globalSessions.Delete(sessionKey)

		log.
			WithField("udid", sessionKey.udid).
			WithField("sessionId", sessionKey.sessionID).
			Debug("Deleted WDA session")
	}()

	globalSessions.Store(sessionKey, session)

	log.
		WithField("udid", sessionKey.udid).
		WithField("sessionId", sessionKey.sessionID).
		Debugf("Requested to start WDA session")

	c.JSON(http.StatusOK, session)
}

// @Summary Получить сессию WebDriverAgent
// @Description Получить сессию WebDriverAgent по sessionId
// @Tags WebDriverAgent
// @Produce json
// @Param sessionId path string true "ID сессии"
// @Success 200 {object} WdaSession
// @Failure 400 {object} GenericResponse
// @Router /wda/session/{sessionId} [get]
func ReadWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	sessionKey := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: sessionID,
	}

	session, loaded := globalSessions.Load(sessionKey)
	if !loaded {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// @Summary Удалить сессию WebDriverAgent
// @Description Удалить сессию WebDriverAgent по sessionId
// @Tags WebDriverAgent
// @Produce json
// @Param sessionId path string true "ID сессии"
// @Success 200 {object} WdaSession
// @Failure 400 {object} GenericResponse
// @Router /wda/session/{sessionId} [delete]
func DeleteWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	sessionKey := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: sessionID,
	}

	session, loaded := globalSessions.Load(sessionKey)
	if !loaded {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	wdaSession, ok := session.(WdaSession)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cast session"})
		return
	}
	wdaSession.stopWda()

	log.
		WithField("udid", sessionKey.udid).
		WithField("sessionId", sessionKey.sessionID).
		Debug("Requested to stop WDA")

	c.JSON(http.StatusOK, session)
}

func FindSessionByUdid(udid string) (WdaSessionKey, *WdaSession, bool) {
	var foundKey WdaSessionKey
	var foundSession *WdaSession
	found := false
	globalSessions.Range(func(key, value any) bool {
		sk, ok := key.(WdaSessionKey)
		if !ok {
			return true
		}
		if sk.udid == udid {
			ws, ok := value.(WdaSession)
			if ok {
				foundKey = sk
				foundSession = &ws
				found = true
				return false // stop iteration
			}
		}
		return true
	})
	return foundKey, foundSession, found
}
