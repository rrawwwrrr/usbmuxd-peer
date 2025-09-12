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
	udid := device.Properties.SerialNumber

	if _, found := f.Get(udid); found {
		return nil, ErrSessionAlreadyExists
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := WdaSession{
		Udid:    udid,
		Config:  config,
		stopWda: cancel,
	}

	go f.runSession(ctx, session, device)

	f.sessions.Store(udid, session)
	UpdateWdaStatus(udid, "created", session.SessionId, "Session created")

	return &session, nil
}

func (f *WdaFactory) runSession(ctx context.Context, session WdaSession, device ios.DeviceEntry) {
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
			WithError(err).
			Error("Failed running WDA")
	}

	session.stopWda()
	fwdMjpeg.Close()
	fwdWda.Close()
	f.sessions.Delete(session.Udid)

	log.WithField("udid", session.Udid).
		Debug("Deleted WDA session")
}

func (f *WdaFactory) Get(udid string) (*WdaSession, bool) {
	val, ok := f.sessions.Load(udid)
	if !ok {
		return nil, false
	}
	session, ok := val.(WdaSession)
	if !ok {
		return nil, false
	}
	return &session, true
}

func (f *WdaFactory) Delete(udid string) (*WdaSession, bool) {
	val, ok := f.sessions.Load(udid)
	if !ok {
		return nil, false
	}
	session := val.(WdaSession)
	session.stopWda()
	f.sessions.Delete(udid)
	UpdateWdaStatus(udid, "deleted", session.SessionId, "Session deleted")
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
// @Description Создать новую сессию WebDriverAgent для указанного устройства (одна сессия на UDID)
// @Tags WebDriverAgent
// @Accept json
// @Produce json
// @Param config body WdaConfig true "Конфигурация WebDriverAgent"
// @Param        udid path string true "UDID устройства"
// @Success 200 {object} WdaSession
// @Failure 400 {object} GenericResponse
// @Router /device/{udid}/wda/session [post]
func CreateWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	var config WdaConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		// fallback на дефолтный конфиг
		config = defaultWdaConfig()
	}

	session, err := wdaFactory.Create(device, config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}

// @Summary Получить активную сессию WebDriverAgent
// @Description Получить активную сессию WebDriverAgent для указанного устройства (по UDID)
// @Tags WebDriverAgent
// @Produce json
// @Success 200 {object} WdaSession
// @Param        udid path string true "UDID устройства"
// @Failure 404 {object} GenericResponse
// @Router /device/{udid}/wda/session [get]
func ReadWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	session, ok := wdaFactory.Get(device.Properties.SerialNumber)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}

// @Summary Удалить сессию WebDriverAgent
// @Description Удалить активную сессию WebDriverAgent для указанного устройства (по UDID)
// @Tags WebDriverAgent
// @Produce json
// @Success 200 {object} WdaSession
// @Param        udid path string true "UDID устройства"
// @Failure 404 {object} GenericResponse
// @Router /device/{udid}/wda/session [delete]
func DeleteWdaSession(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	session, ok := wdaFactory.Delete(device.Properties.SerialNumber)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, session)
}

func defaultWdaConfig() WdaConfig {
	bundleID := os.Getenv("WDA_BUNDLE_ID")
	if bundleID == "" {
		//bundleID = "com.facebook.WebDriverAgentRunner.xctrunner"
		bundleID = "com.x5.WebDriverAgentRunner.xctrunner"
	}
	return WdaConfig{
		BundleID:     bundleID,
		TestbundleID: bundleID,
		XCTestConfig: "WebDriverAgentRunner.xctest",
		Args:         []string{},
		Env: map[string]interface{}{
			"MJPEG_SERVER_PORT":         "8001",
			"USE_PORT":                  "8100",
			"UITEST_DISABLE_ANIMATIONS": "YES",
		},
	}
}
