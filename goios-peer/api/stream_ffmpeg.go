package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/gin-gonic/gin"
)

// StreamRequest структура запроса для старта стрима
type StreamRequest struct {
	URL  string `json:"url" example:"192.168.1.50"`
	Port int    `json:"port" example:"5004"`
}

var (
	mu  sync.Mutex
	cmd *exec.Cmd
)

// startStream запускает ffmpeg для ретрансляции MJPEG -> H264 -> RTP
func startStream(host string, port int) error {
	stopStream()

	mjpegURL := "http://127.0.0.1:8001"

	args := []string{
		"-v", "verbose",
		"-re",
		"-fflags", "+genpts",
		"-r", "25",
		"-i", mjpegURL,
		"-an",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-level", "3.1",
		"-g", "25", "-keyint_min", "25", "-sc_threshold", "0",
		"-b:v", "1500k", "-maxrate", "1500k", "-bufsize", "1500k",
		"-x264-params", "nal-hrd=cbr:repeat-headers=1",
		"-f", "rtp", "-payload_type", "96",
		fmt.Sprintf("rtp://%s:%d?pkt_size=1200", host, port),
	}

	c := exec.Command("ffmpeg", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Start(); err != nil {
		return err
	}

	mu.Lock()
	cmd = c
	mu.Unlock()

	go func() {
		_ = c.Wait()
		stopStream()
	}()

	return nil
}

func stopStream() {
	mu.Lock()
	defer mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		cmd = nil
	}
}

// StartStream godoc
// @Summary Запуск стрима
// @Description Стартует ретрансляцию MJPEG -> H264 -> RTP
// @Tags stream
// @Accept json
// @Produce plain
// @Param request body StreamRequest true "Данные для запуска"
// @Success 200 {string} string "Stream started"
// @Failure 400 {string} string "Bad request"
// @Failure 500 {string} string "Internal error"
// @Router /start [post]
func StartStream(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

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

	wda := NewWdaFactory()
	wda.Create(device, config)
	var req StreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	if err := startStream(req.URL, req.Port); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.String(http.StatusOK, "Stream started to %s:%d", req.URL, req.Port)
}

// StopStream godoc
// @Summary Остановка стрима
// @Description Останавливает текущий ffmpeg процесс
// @Tags stream
// @Produce plain
// @Success 200 {string} string "Stream stopped"
// @Router /stop [post]
func StopStream(c *gin.Context) {
	stopStream()
	c.String(http.StatusOK, "Stream stopped")
}

// StatusStream godoc
// @Summary Статус стрима
// @Description Возвращает состояние стриминга
// @Tags stream
// @Produce plain
// @Success 200 {string} string "running|stopped"
// @Router /status [get]
func StatusStream(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		c.String(http.StatusOK, "running")
	} else {
		c.String(http.StatusOK, "stopped")
	}
}
