package api

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	ProxySourceURL   = "http://localhost:8001"
	MjpegBoundary    = "--BoundaryString"
	MjpegHeader      = "Content-Type: image/jpeg"
	MjpegFrameHeader = "--BoundaryString\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n"
	MjpegFrameFooter = "\r\n\r\n"
)

type ProxyManager struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	running bool
	stop    chan struct{}
}

func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		clients: make(map[chan []byte]struct{}),
	}
}

func (p *ProxyManager) AddClient() chan []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan []byte, 10)
	p.clients[ch] = struct{}{}
	if !p.running {
		p.start()
	}
	return ch
}

func (p *ProxyManager) RemoveClient(ch chan []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, ch)
	close(ch)
	if len(p.clients) == 0 && p.running {
		p.stopStreaming()
	}
}

func (p *ProxyManager) start() {
	p.stop = make(chan struct{})
	p.running = true
	go p.streamLoop()
}

func (p *ProxyManager) stopStreaming() {
	close(p.stop)
	p.running = false
}

func (p *ProxyManager) streamLoop() {
	for {
		select {
		case <-p.stop:
			return
		default:
			// Подключаемся к источнику MJPEG
			resp, err := http.Get(ProxySourceURL)
			if err != nil {
				fmt.Printf("Ошибка подключения к источнику: %v\n", err)
				continue
			}
			defer resp.Body.Close()

			reader := bufio.NewReader(resp.Body)
			boundary := []byte(MjpegBoundary)
			for {
				select {
				case <-p.stop:
					return
				default:
					// Ищем начало кадра
					line, err := reader.ReadBytes('\n')
					if err != nil {
						fmt.Printf("Ошибка чтения: %v\n", err)
						break
					}
					if !bytes.Contains(line, boundary) {
						continue
					}

					// Читаем заголовки кадра
					headers := make(map[string]string)
					for {
						h, err := reader.ReadString('\n')
						if err != nil {
							break
						}
						h = h[:len(h)-1]
						if len(h) == 0 || h == "\r" {
							break
						}
						var key, value string
						fmt.Sscanf(h, "%s: %s", &key, &value)
						headers[key] = value
					}

					// Читаем тело кадра
					var buf bytes.Buffer
					for {
						b, err := reader.ReadByte()
						if err != nil {
							break
						}
						buf.WriteByte(b)
						if buf.Len() > 2 && buf.Bytes()[buf.Len()-2] == 0xFF && buf.Bytes()[buf.Len()-1] == 0xD9 {
							// JPEG EOF
							break
						}
					}
					jpegBytes := buf.Bytes()

					// Рассылаем всем клиентам
					p.mu.Lock()
					for ch := range p.clients {
						select {
						case ch <- jpegBytes:
						default:
						}
					}
					p.mu.Unlock()
				}
			}
		}
	}
}

var proxyManager = NewProxyManager()

func MJPEGProxyHandler(c *gin.Context) {
	ch := proxyManager.AddClient()
	defer proxyManager.RemoveClient(ch)

	w := c.Writer

	for frame := range ch {
		_, err := w.Write([]byte(fmt.Sprintf(MjpegFrameHeader, len(frame))))
		if err != nil {
			break
		}
		_, err = w.Write(frame)
		if err != nil {
			break
		}
		_, err = w.Write([]byte(MjpegFrameFooter))
		if err != nil {
			break
		}
		w.Flush()
	}
}
