package api

import (
	"context"
	"errors"
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

var (
	ErrSessionAlreadyExists = errors.New("session already exists for this device")
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

func (s *WdaSession) Write(p []byte) (n int, err error) {
	log.WithField("udid", s.Udid).
		WithField("sessionId", s.SessionId).
		Debugf("WDA_LOG %s", p)
	return len(p), nil
}

/* ------------------ Factory ------------------ */

type WdaFactory struct {
	sessions sync.Map
}

func NewWdaFactory() *WdaFactory {
	return &WdaFactory{}
}

func (f *WdaFactory) Create(device ios.DeviceEntry, config WdaConfig) (*WdaSession, error) {
	if _, existing, found := f.FindByUdid(device.Properties.SerialNumber); found {
		return existing, ErrSessionAlreadyExists
	}

	sessionKey := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: uuid.New().String(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := WdaSession{
		Udid:      sessionKey.udid,
		SessionId: sessionKey.sessionID,
		Config:    config,
		stopWda:   cancel,
	}

	go f.runSession(ctx, sessionKey, session, device)

	f.sessions.Store(sessionKey, session)

	return &session, nil
}

func (f *WdaFactory) runSession(ctx context.Context, key WdaSessionKey, session WdaSession, device ios.DeviceEntry) {
	// MJPEG port forward
	mjpegPortStr, _ := session.Config.Env["MJPEG_SERVER_PORT"].(string)
	mjpegPort, _ := strconv.ParseUint(mjpegPortStr, 10, 16)
	fwdMjpeg, _ := forward.Forward(device, uint16(mjpegPort), uint16(mjpegPort))

	// WDA port forward
	usePortStr, _ := session.Config.Env["USE_PORT"].(string)
	usePort, _ := strconv.ParseUint(usePortStr, 10, 16)
	fwdWda, _ := forward.Forward(device, uint16(usePort), uint16(usePort))

	// Запуск WDA
	_, err := testmanagerd.RunTestWithConfig(ctx, testmanagerd.TestConfig{
		BundleId:           session.Config.BundleID,
		TestRunnerBundleId: session.Config.TestbundleID,
		XctestConfigName:   session.Config.XCTestConfig,
		Env:                session.Config.Env,
		Args:               session.Config.Args,
		Device:             device,
		Listener:           testmanagerd.NewTestListener(&session, &session, os.TempDir()),
	})
	if err != nil {
		log.WithField("udid", session.Udid).
			WithField("sessionId", session.SessionId).
			WithError(err).
			Error("Failed running WDA")
	}

	session.stopWda()
	fwdMjpeg.Close()
	fwdWda.Close()
	f.sessions.Delete(key)

	log.WithField("udid", session.Udid).
		WithField("sessionId", session.SessionId).
		Debug("Deleted WDA session")
}

func (f *WdaFactory) Get(key WdaSessionKey) (*WdaSession, bool) {
	val, ok := f.sessions.Load(key)
	if !ok {
		return nil, false
	}
	session, ok := val.(WdaSession)
	if !ok {
		return nil, false
	}
	return &session, true
}

func (f *WdaFactory) Delete(key WdaSessionKey) (*WdaSession, bool) {
	val, ok := f.sessions.Load(key)
	if !ok {
		return nil, false
	}
	session := val.(WdaSession)
	session.stopWda()
	f.sessions.Delete(key)
	return &session, true
}

func (f *WdaFactory) FindByUdid(udid string) (WdaSessionKey, *WdaSession, bool) {
	var foundKey WdaSessionKey
	var foundSession *WdaSession
	found := false
	f.sessions.Range(func(k, v any) bool {
		sk, ok := k.(WdaSessionKey)
		if !ok {
			return true
		}
		if sk.udid == udid {
			if ws, ok := v.(WdaSession); ok {
				foundKey = sk
				foundSession = &ws
				found = true
				return false
			}
		}
		return true
	})
	return foundKey, foundSession, found
}

/* ------------------ API ------------------ */

var wdaFactory = NewWdaFactory()

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

	var config WdaConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		// fallback на дефолтный конфиг
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

	session, err := wdaFactory.Create(device, config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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
// @Example request {"config":{"bundleId":"com.facebook.WebDriverAgentRunner.xctrunner","testbundleId":"com.facebook.WebDriverAgentRunner.xctrunner","xctestConfig":"WebDriverAgentRunner.xctest","args":[],"env":{"MJPEG_SERVER_PORT":"8001","USE_PORT":"8100","UITEST_DISABLE_ANIMATIONS":"YES"}}}
// @Example response {"config":{"bundleId":"com.facebook.WebDriverAgentRunner.xctrunner","testbundleId":"com.facebook.WebDriverAgentRunner.xctrunner","xctestConfig":"WebDriverAgentRunner.xctest","args":[],"env":{"MJPEG_SERVER_PORT":"8001","USE_PORT":"8100","UITEST_DISABLE_ANIMATIONS":"YES"}},"sessionId":"12345678-90ab-cdef-1234-567890abcdef","udid":"00008020-001C195E0A88002E"}
func ReadWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	sessionID := c.Param("sessionId")

	key := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: sessionID,
	}

	session, ok := wdaFactory.Get(key)
	if !ok {
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

	key := WdaSessionKey{
		udid:      device.Properties.SerialNumber,
		sessionID: sessionID,
	}

	session, ok := wdaFactory.Delete(key)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}
