package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// StreamRequest структура запроса для старта стрима
type StreamRequest struct {
	URL  string `json:"url" example:"192.168.1.50"`
	Port int    `json:"port" example:"5004"`
	Gw   string `json:"gw" example:"4000"`
}

var (
	mu  sync.Mutex
	cmd *exec.Cmd
)

// startStream запускает ffmpeg для ретрансляции MJPEG -> H264 -> RTP
func startStream(host string, port int, gwPort string, mjpegHost string, mjpegPort int) error {

	mjpegURL := fmt.Sprintf("http://%s:%d", mjpegHost, mjpegPort)
	log.WithField("ssrc", port).Info("Start stream")
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
		"-an",

		// --- ВЫХОД: видео только, стабильный CBR, низкая задержка ---

		"-map", "0:v",
		"-c:v", "libx264",
		"-preset", "medium",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-level", "4.0",
		"-g", "25", "-keyint_min", "25", "-sc_threshold", "0",
		"-b:v", "1500k", "-maxrate", "1500k", "-bufsize", "1500k",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-f", "rtp", "-payload_type", "96",
		"-ssrc", strconv.Itoa(port),
		fmt.Sprintf("rtp://%s:%s?pkt_size=1200", host, gwPort),

		"-map", "0:v",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-level", "4.0",
		"-f", "mp4",
		"-movflags", "+frag_keyframe+empty_moov",
		"-flush_packets", "1",
		"-threads", "1",
		"-y",
		"recording.mp4",
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
	wdaConfig := defaultWdaConfig(device)
	wdaFactory.Create(device, wdaConfig)

	if err := waitForMJPEG("http://127.0.0.1:8001", 10*time.Second); err != nil {
		log.Error(err)
	}
	if err := startStream(req.URL, req.Port, req.Gw, "127.0.0.1", 8001); err != nil {
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

// DownloadRecording godoc
// @Summary Скачать запись стрима
// @Description Возвращает файл стриминга для скачивания
// @Tags stream
// @Produce video/mp4
// @Param        udid path string true "UDID устройства"
// @Success 200 {file} file "Recording file"
// @Failure 404 {string} string "File not found"
// @Failure 500 {string} string "Internal error"
// @Router /device/{udid}/stream/recording [get]
func DownloadRecording(c *gin.Context) {
	filePath, err := getRecordingFilePath()
	if err != nil {
		log.WithError(err).Error("Failed to get recording file")
		c.String(http.StatusNotFound, "Recording file not found")
		return
	}

	// Устанавливаем заголовки для скачивания
	c.Header("Content-Disposition", "attachment; filename=\"recording.mp4\"")
	c.Header("Content-Type", "video/mp4")
	c.File(filePath)
}

// PreviewRecording godoc
// @Summary Просмотреть запись стрима
// @Description Возвращает файл стриминга для просмотра в браузере
// @Tags stream
// @Produce video/mp4
// @Param        udid path string true "UDID устройства"
// @Success 200 {file} file "Recording file"
// @Failure 404 {string} string "File not found"
// @Failure 500 {string} string "Internal error"
// @Router /device/{udid}/stream/preview [get]
func PreviewRecording(c *gin.Context) {
	filePath, err := getRecordingFilePath()
	if err != nil {
		log.WithError(err).Error("Failed to get recording file")
		c.String(http.StatusNotFound, "Recording file not found")
		return
	}

	// Устанавливаем заголовки для просмотра в браузере
	c.Header("Content-Disposition", "inline; filename=\"recording.mp4\"")
	c.Header("Content-Type", "video/mp4")
	c.File(filePath)
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

// getRecordingFilePath возвращает путь к файлу записи
func getRecordingFilePath() (string, error) {
	filePath := "/app/recording.mp4"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("recording file not found: %s", filePath)
	}
	return filePath, nil
}
