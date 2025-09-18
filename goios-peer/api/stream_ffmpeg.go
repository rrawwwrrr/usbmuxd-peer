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
func startStream(host string, port int, mjpegHost string, mjpegPort int) error {

	mjpegURL := fmt.Sprintf("http://%s:%d", mjpegHost, mjpegPort)

	args := []string{
		// --- Основные настройки потока ---
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-reconnect_at_eof", "1",
		"-i", mjpegURL,
		"-v", "verbose", // подробные логи в файл
		"-re", "-stream_loop", "-1",
		"-fflags", "+genpts",
		"-r", "25",
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
		//// --- Ключевые флаги для низкой задержки и дропа кадров ---
		//"-fflags", "+genpts+discardcorrupt+nobuffer+flush_packets",
		//"-flags", "low_delay",
		//"-strict", "experimental", // если используешь experimental кодеки
		//
		//// Уменьшаем размеры буферов
		//"-rtbufsize", "64k", // маленький буфер ввода
		//"-probesize", "32", // быстрее стартует
		//
		//"-r", "25",
		//"-i", mjpegURL,
		//
		//// --- Видеофильтр (время) ---
		//"-an", // без аудио
		//
		//// --- Кодирование: максимум скорости, минимум буфера ---
		//"-c:v", "libx264",
		//"-preset", "ultrafast", // вместо veryfast — ещё быстрее
		//"-tune", "zerolatency", // критично для low-latency
		//"-pix_fmt", "yuv420p",
		//"-profile:v", "baseline", // лучше для совместимости
		//"-level", "3.1",
		//
		//// Битрейт (можно снизить, если сеть слабая)
		//"-b:v", "1000k",
		//"-maxrate", "1000k",
		//"-bufsize", "500k", // маленький буфер декодера
		//
		//// Ключевые кадры каждые 2 секунды (~50 при 25fps), но не реже чем нужно
		//"-g", "25",
		//"-keyint_min", "25",
		//"-sc_threshold", "0", // всегда делай I-кадры по расписанию
		//"-forced-idr", "1",
		//
		//// Отключаем сложные оптимизации, которые замедляют
		//"-x264-params", "nal-hrd=cbr:repeat-headers=1:no-mbtree=1:vbv-maxrate=1000:vbv-bufsize=500:sliced-threads=1:sync-lookahead=0",
		//
		//// --- Самое важное: форсировать немедленную отправку пакетов ---
		//"-f", "rtp",
		//"-payload_type", "96",
		//"-flush_packets", "1", // ! Очень важно: пакеты отправляются немедленно
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
