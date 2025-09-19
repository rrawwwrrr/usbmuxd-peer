package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type StreamConfig struct {
	VideoCodec   string `json:"video_codec"` // например, "libx264"
	Preset       string `json:"preset"`      // "veryfast", "ultrafast"...
	Tune         string `json:"tune"`        // "zerolatency"
	PixFmt       string `json:"pix_fmt"`     // "yuv420p"
	Profile      string `json:"profile"`     // "baseline"
	Level        string `json:"level"`       // "3.1"
	GopSize      *int   `json:"gop_size"`    // int pointer — может быть nil
	KeyintMin    *int   `json:"keyint_min"`
	SCRThreshold *int   `json:"sc_threshold"` // например, 0
	Bitrate      string `json:"bitrate"`      // "1500k"
	MaxRate      string `json:"maxrate"`      // может быть пустым
	BufSize      string `json:"bufsize"`      // может быть пустым
	X264Params   string `json:"x264_params"`  // дополнительные параметры
	PayloadType  *int   `json:"payload_type"` // например, 96
	FMT          string `json:"format"`       // "rtp"
	PacketSize   *int   `json:"pkt_size"`     // для RTP
}

// StreamRequest структура запроса для старта стрима
type StreamRequest struct {
	URL  string `json:"url" example:"192.168.1.50"`
	Port int    `json:"port" example:"5004"`

	Preset      string `json:"preset"`
	Bitrate     string `json:"bitrate"`
	MaxRate     string `json:"maxrate"`
	BufSize     string `json:"bufsize"`
	GopSize     *int   `json:"gop_size"` // указатель — может быть null
	KeyintMin   *int   `json:"keyint_min"`
	Level       string `json:"level"`
	Profile     string `json:"profile"`
	PixFmt      string `json:"pix_fmt"`
	Tune        string `json:"tune"`
	X264Params  string `json:"x264_params"`
	PayloadType *int   `json:"payload_type"`
	PacketSize  *int   `json:"pkt_size"`
	OverlayTime bool   `json:"overlay_time"`
}

var (
	mu  sync.Mutex
	cmd *exec.Cmd
)

// startStream запускает ffmpeg для ретрансляции MJPEG -> H264 -> RTP
func startStream(host string, port int, mjpegHost string, mjpegPort int) error {

	mjpegURL := fmt.Sprintf("http://%s:%d", mjpegHost, mjpegPort)
	args := []string{
		// --- Настройки ВХОДНОГО потока (перед -i) ---
		"-rw_timeout", "2000000", // 2s: если вход завис — быстро отвалиться
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_on_network_error", "1",

		"-v", "verbose",
		"-re", "-stream_loop", "-1",
		"-fflags", "+genpts",
		"-r", "25",

		"-i", mjpegURL, // источник

		// --- ВЫХОД: видео только, стабильный CBR, низкая задержка ---
		"-an",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-level", "3.1",
		"-g", "25", "-keyint_min", "25", "-sc_threshold", "0",
		"-b:v", "1500k", "-maxrate", "1500k", "-bufsize", "1500k",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-x264-params", "bframes=0:bpyramid=0:nal-hrd=cbr:repeat-headers=1:threads=4:sync-lookahead=0:rc-lookahead=0",
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
		stopStream("")
	}()

	return nil
}

func stopStream(udid string) {

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
// @Param        udid path string true "UDID устройства"
// @Success 200 {string} string "Stream started"
// @Failure 400 {string} string "Bad request"
// @Failure 500 {string} string "Internal error"
// @Router /device/{udid}/stream/start [post]
func StartStream(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	var req StreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	wdaConfig := defaultWdaConfig()
	wdaFactory.Create(device, wdaConfig)

	if err := waitForMJPEG("http://127.0.0.1:8001", 10*time.Second); err != nil {
		log.Error(err)
	}
	if err := startStream(req.URL, req.Port, "127.0.0.1", 8001); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		wdaFactory.Delete(device.Properties.SerialNumber)
		return
	}

	c.String(http.StatusOK, "Stream started to %s:%d", req.URL, req.Port)
}

// StopStream godoc
// @Summary Остановка стрима
// @Description Останавливает текущий ffmpeg процесс
// @Tags stream
// @Produce plain
// @Param        udid path string true "UDID устройства"
// @Success 200 {string} string "Stream stopped"
// @Router /device/{udid}/stream/stop [post]
func StopStream(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	// Останавливаем ffmpeg процесс (твоя логика)
	stopStream(device.Properties.SerialNumber)

	// Пытаемся завершить WDA-сессию
	if session, ok := wdaFactory.Delete(device.Properties.SerialNumber); ok {
		log.WithField("udid", session.Udid).Info("WDA session stopped with stream stop")
	}

	c.String(http.StatusOK, "Stream stopped")
}

// StatusStream godoc
// @Summary Статус стрима
// @Description Возвращает состояние стриминга
// @Tags stream
// @Produce plain
// @Param        udid path string true "UDID устройства"
// @Success 200 {string} string "running|stopped"
// @Router /device/{udid}/stream/status [get]
func StatusStream(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		c.String(http.StatusOK, "running")
	} else {
		c.String(http.StatusOK, "stopped")
	}
}
func waitForMJPEG(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(address)
		if err == nil {
			resp.Body.Close()
			ct := resp.Header.Get("Content-Type")
			if strings.HasPrefix(ct, "multipart/x-mixed-replace") {
				return nil // MJPEG сервер готов
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for MJPEG at %s", address)
}
