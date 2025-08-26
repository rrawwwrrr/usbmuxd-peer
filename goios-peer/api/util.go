package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type GenericResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// GetVersion читает содержимое файла version.txt и возвращает его.
// Если файл не удаётся прочитать, возвращает "не удалось прочитать версию"
func GetVersion() string {
	version, err := os.ReadFile("version.txt")
	if err != nil {
		return "could not read version"
	}
	return string(version)
}

func MustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

var timeFormat = "02/Jan/2006:15:04:05 -0700"

// взято с https://github.com/toorop/gin-logrus/blob/master/logger.go
func MyLogger(logger logrus.FieldLogger, notLogged ...string) gin.HandlerFunc {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknow"
	}

	var skip map[string]struct{}

	if length := len(notLogged); length > 0 {
		skip = make(map[string]struct{}, length)

		for _, p := range notLogged {
			skip[p] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		// other handler can change c.Path so:
		path := c.Request.URL.Path
		start := time.Now()
		c.Next()
		stop := time.Since(start)
		latency := int(math.Ceil(float64(stop.Nanoseconds()) / 1000000.0))
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		clientUserAgent := c.Request.UserAgent()
		referer := c.Request.Referer()
		dataLength := c.Writer.Size()
		if dataLength < 0 {
			dataLength = 0
		}

		if _, ok := skip[path]; ok {
			return
		}

		entry := logger.WithFields(logrus.Fields{
			"hostname":   hostname,
			"statusCode": statusCode,
			"latency":    latency, // time to process
			"clientIP":   clientIP,
			"method":     c.Request.Method,
			"path":       path,
			"referer":    referer,
			"dataLength": dataLength,
			"userAgent":  clientUserAgent,
		})

		if len(c.Errors) > 0 {
			entry.Error(c.Errors.ByType(gin.ErrorTypePrivate).String())
		} else {
			msg := fmt.Sprintf("%s - %s [%s] \"%s %s\" %d %d \"%s\" \"%s\" (%dms)", clientIP, hostname, time.Now().Format(timeFormat), c.Request.Method, path, statusCode, dataLength, referer, clientUserAgent, latency)
			if statusCode >= http.StatusInternalServerError {
				entry.Error(msg)
			} else if statusCode >= http.StatusBadRequest {
				entry.Warn(msg)
			} else {
				entry.Info(msg)
			}
		}
	}
}
