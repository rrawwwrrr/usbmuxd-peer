package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// === Структуры ===

type StreamRequest2 struct {
	URL  string `json:"url" binding:"required"`
	Port int    `json:"port" binding:"required,gt=0"`

	Preset       string `json:"preset"`
	Tune         string `json:"tune"`
	Bitrate      string `json:"bitrate"`
	MaxRate      string `json:"maxrate"`
	BufSize      string `json:"bufsize"`
	GopSize      *int   `json:"gop_size"`
	KeyintMin    *int   `json:"keyint_min"`
	SCRThreshold *int   `json:"sc_threshold"`
	Level        string `json:"level"`
	Profile      string `json:"profile"`
	PixFmt       string `json:"pix_fmt"`
	X264Params   string `json:"x264_params"`
	PayloadType  *int   `json:"payload_type"`
	PacketSize   *int   `json:"pkt_size"`
	Format       string `json:"format"`
	OverlayTime  bool   `json:"overlay_time"`
}

type StreamConfig2 struct {
	VideoCodec   string
	Preset       string
	Tune         string
	PixFmt       string
	Profile      string
	Level        string
	GopSize      *int
	KeyintMin    *int
	SCRThreshold *int
	Bitrate      string
	MaxRate      string
	BufSize      string
	X264Params   string
	PayloadType  *int
	Format       string
	PacketSize   *int
	OverlayTime  bool
}

func parseFloatFromBitrate(s string) float64 {
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`(\d+\.?\d*)([km]?)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0
	}
	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	switch matches[2] {
	case "k":
		num *= 1000
	case "m":
		num *= 1000000
	}
	return num
}

// === Сборка аргументов ffmpeg ===

func buildFFmpegArgs(config StreamConfig2, mjpegURL, host string, port int) []string {
	var args []string

	// --- Вход ---
	args = append(args,
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-reconnect_at_eof", "1",
		"-fflags", "+genpts+discardcorrupt+nobuffer",
		"-flags", "low_delay",
		"-rtbufsize", "64k",
		"-probesize", "32",
		"-r", "25",
		"-re",
		"-stream_loop", "-1",
		"-i", mjpegURL,
	)

	// --- Видео кодек ---
	codec := "libx264"
	args = append(args, "-c:v", codec)

	// --- Preset ---
	if config.Preset != "" {
		args = append(args, "-preset", config.Preset)
	} else {
		args = append(args, "-preset", "veryfast")
	}

	// --- Tune ---
	if config.Tune != "" {
		args = append(args, "-tune", config.Tune)
	} else {
		args = append(args, "-tune", "zerolatency")
	}

	// --- Pixfmt ---
	if config.PixFmt != "" {
		args = append(args, "-pix_fmt", config.PixFmt)
	} else {
		args = append(args, "-pix_fmt", "yuv420p")
	}

	// --- Profile ---
	if config.Profile != "" {
		args = append(args, "-profile:v", config.Profile)
	} else {
		args = append(args, "-profile:v", "baseline")
	}

	// --- Level ---
	if config.Level != "" {
		args = append(args, "-level", config.Level)
	} else {
		args = append(args, "-level", "3.1")
	}

	// --- GOP ---
	gop := 25
	if config.GopSize != nil {
		gop = *config.GopSize
	}
	args = append(args, "-g", fmt.Sprintf("%d", gop))

	keyint := gop
	if config.KeyintMin != nil {
		keyint = *config.KeyintMin
	}
	args = append(args, "-keyint_min", fmt.Sprintf("%d", keyint))

	scThr := 0
	if config.SCRThreshold != nil {
		scThr = *config.SCRThreshold
	}
	args = append(args, "-sc_threshold", fmt.Sprintf("%d", scThr))

	// --- Битрейт ---
	bitrate := "1500k"
	if config.Bitrate != "" {
		bitrate = config.Bitrate
	}
	args = append(args, "-b:v", bitrate)

	maxrate := bitrate
	if config.MaxRate != "" {
		maxrate = config.MaxRate
	}
	args = append(args, "-maxrate", maxrate)

	bufsize := bitrate
	if config.BufSize != "" {
		bufsize = config.BufSize
	} else {
		if v := parseFloatFromBitrate(bitrate); v > 0 {
			bufsize = fmt.Sprintf("%.0fk", v/1000)
		}
	}
	args = append(args, "-bufsize", bufsize)

	// --- x264 params ---
	x264params := "nal-hrd=cbr:repeat-headers=1:no-mbtree=1:sliced-threads=1:sync-lookahead=0"
	if config.X264Params != "" {
		x264params = config.X264Params
	}
	args = append(args, "-x264-params", x264params)

	// --- Формат вывода ---
	format := "rtp"
	if config.Format != "" {
		format = config.Format
	}
	args = append(args, "-f", format)

	// --- Payload Type ---
	pt := 96
	if config.PayloadType != nil {
		pt = *config.PayloadType
	}
	args = append(args, "-payload_type", fmt.Sprintf("%d", pt))

	// --- RTP URL ---
	pktSize := 1200
	if config.PacketSize != nil {
		pktSize = *config.PacketSize
	}
	rtpURL := fmt.Sprintf("rtp://%s:%d?pkt_size=%d", host, port, pktSize)
	args = append(args, rtpURL)

	return args
}

// === Управление стримом ===

func startStreamParam(config StreamConfig2, host string, port int, mjpegHost string, mjpegPort int) error {
	mu.Lock()
	defer mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		return fmt.Errorf("stream already running")
	}

	mjpegURL := fmt.Sprintf("http://%s:%d", mjpegHost, mjpegPort)
	args := buildFFmpegArgs(config, mjpegURL, host, port)

	c := exec.Command("ffmpeg", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	log.Printf("Starting FFmpeg: ffmpeg %v", args)

	if err := c.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	cmd = c

	go func() {
		err := c.Wait()
		mu.Lock()
		cmd = nil
		mu.Unlock()
		if err != nil {
			log.Printf("Stream stopped with error: %v", err)
		} else {
			log.Println("Stream stopped gracefully")
		}
	}()

	return nil
}

// === HTTP Handler ===

func StartStream2(c *gin.Context) {
	var req StreamRequest2
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	// Здесь можно подключить wdaFactory и device, если нужно
	// Например: device := c.MustGet("device").(DeviceEntry)

	if err := waitForMJPEG("http://127.0.0.1:8001", 10*time.Second); err != nil {
		log.Printf("MJPEG not available: %v", err)
		c.String(http.StatusServiceUnavailable, "MJPEG stream not ready: %v", err)
		return
	}

	config := StreamConfig2{
		Preset:       req.Preset,
		Tune:         req.Tune,
		Bitrate:      req.Bitrate,
		MaxRate:      req.MaxRate,
		BufSize:      req.BufSize,
		GopSize:      req.GopSize,
		KeyintMin:    req.KeyintMin,
		SCRThreshold: req.SCRThreshold,
		Level:        req.Level,
		Profile:      req.Profile,
		PixFmt:       req.PixFmt,
		X264Params:   req.X264Params,
		PayloadType:  req.PayloadType,
		PacketSize:   req.PacketSize,
		Format:       req.Format,
		OverlayTime:  req.OverlayTime,
	}

	if err := startStreamParam(config, req.URL, req.Port, "127.0.0.1", 8001); err != nil {
		log.Printf("Failed to start stream: %v", err)
		c.String(http.StatusInternalServerError, "Start failed: %v", err)
		return
	}

	c.String(http.StatusOK, "Stream started to %s:%d", req.URL, req.Port)
}
