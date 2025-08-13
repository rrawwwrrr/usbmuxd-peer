package api

import (
	"bufio"
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	mjpegFrameFooter = "\r\n\r\n"
	mjpegFrameHeader = "--BoundaryString\r\nContent-type: image/jpg\r\nContent-Length: %d\r\n\r\n"
)

// StreamManager управляет трансляцией кадров и клиентами для одного устройства/пути.
type StreamManager struct {
	mu        sync.Mutex
	consumers map[chan []byte]struct{}
	running   bool
	stop      chan struct{}
	device    ios.DeviceEntry
	conn      *instruments.ScreenshotService
}

// Создаёт новый StreamManager для конкретного устройства.
func NewStreamManager(device ios.DeviceEntry) *StreamManager {
	return &StreamManager{
		consumers: make(map[chan []byte]struct{}),
		device:    device,
	}
}

// Добавляет клиента и запускает поток при первом клиенте.
func (s *StreamManager) AddClient() chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan []byte, 10)
	s.consumers[ch] = struct{}{}
	if !s.running {
		s.startStreaming()
	}
	return ch
}

// Удаляет клиента и останавливает поток, если клиентов не осталось.
func (s *StreamManager) RemoveClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.consumers, ch)
	close(ch)
	if len(s.consumers) == 0 && s.running {
		s.stopStreaming()
	}
}

// Запускает поток скриншотов для всех клиентов этого StreamManager.
func (s *StreamManager) startStreaming() {
	s.stop = make(chan struct{})
	s.running = true

	conn, err := instruments.NewScreenshotService(s.device)
	if err != nil {
		log.Errorf("failed to start screenshot service: %v", err)
		s.running = false
		return
	}
	s.conn = conn

	go func() {
		var opt jpeg.Options
		opt.Quality = 80
		for {
			select {
			case <-s.stop:
				return
			default:
				start := time.Now()
				pngBytes, err := s.conn.TakeScreenshot()
				if err != nil {
					log.Warnf("Screenshot failed: %v", err)
					time.Sleep(1 * time.Second)
					continue
				}
				img, err := png.Decode(bytes.NewReader(pngBytes))
				if err != nil {
					log.Warnf("failed decoding png %v", err)
					continue
				}
				var b bytes.Buffer
				foo := bufio.NewWriter(&b)
				err = jpeg.Encode(foo, img, &opt)
				if err != nil {
					log.Warnf("failed encoding jpg %v", err)
					continue
				}
				foo.Flush()
				jpg := b.Bytes()

				elapsed := time.Since(start)
				log.Debugf("frame prepared in %fs", elapsed.Seconds())

				s.mu.Lock()
				for ch := range s.consumers {
					// Не блокируем, если клиент не успевает получать кадры
					select {
					case ch <- jpg:
					default:
					}
				}
				s.mu.Unlock()

				time.Sleep(100 * time.Millisecond) // ~10fps
			}
		}
	}()
}

// Останавливает поток скриншотов.
func (s *StreamManager) stopStreaming() {
	close(s.stop)
	s.running = false
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// Глобальная map для StreamManager'ов по уникальному пути.
var (
	streamManagersMu sync.Mutex
	streamManagers   = make(map[string]*StreamManager)
)

// Возвращает StreamManager для пути (создаёт, если не существует).
func getStreamManager(path string, device ios.DeviceEntry) *StreamManager {
	streamManagersMu.Lock()
	defer streamManagersMu.Unlock()
	sm, ok := streamManagers[path]
	if !ok {
		sm = NewStreamManager(device)
		streamManagers[path] = sm
	}
	return sm
}

// MJPEGStreamHandler godoc
// @Summary      MJPEG Stream
// @Description  Возвращает MJPEG-поток скриншотов с iOS-устройства.
// @Tags         stream
// @Produce      multipart/x-mixed-replace
// @Param        device_id  path      string  true  "ID устройства"
// @Success      200  {string}  string  "stream"
// @Header       200  {string}  Content-Type "multipart/x-mixed-replace; boundary=--BoundaryString"
// @Header       200  {string}  Cache-Control "no-cache, private"
// @Header       200  {string}  Pragma "no-cache"
// @Header       200  {string}  Access-Control-Allow-Origin "*"
// @Router       /screenstream [get]
func MJPEGStreamHandler(c *gin.Context) {
	path := c.Request.URL.Path
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	sm := getStreamManager(path, device)
	log.Infof("starting mjpeg stream for client on path %s", path)
	ch := sm.AddClient()
	defer sm.RemoveClient(ch)

	writer := c.Writer

	for jpg := range ch {
		_, err := writer.Write([]byte(fmt.Sprintf(mjpegFrameHeader, len(jpg))))
		if err != nil {
			break
		}
		_, err = writer.Write(jpg)
		if err != nil {
			break
		}
		_, err = writer.Write([]byte(mjpegFrameFooter))
		if err != nil {
			break
		}
		writer.Flush()
	}
	log.Infof("client disconnected from %s", path)
}
