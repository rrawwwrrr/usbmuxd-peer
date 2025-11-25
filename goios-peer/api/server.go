package api

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"

	"goios-peer/docs"
)

func StartRestAPI() {
	privateMux := http.NewServeMux()
	registry := prometheus.NewRegistry()

	privateMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	privateMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	go func() {
		listen := ":8888"
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			log.Printf("Failed to listen on %s: %v", listen, err)
			return
		}

		fmt.Printf("Starting private server on %s\n", listen)
		if err := http.Serve(listener, privateMux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server failed: %v", err)
		}
	}()

	basePath := "/api/v1"
	router := gin.Default()
	log := logrus.New()
	myfile, _ := os.Create("go-ios.log")
	gin.DefaultWriter = io.MultiWriter(myfile, os.Stdout)
	TunnelStart()
	router.Use(MyLogger(log), gin.Recovery())
	docs.SwaggerInfo.BasePath = basePath
	websocketRoutes(router)
	downLoadRoutes(router)
	v1 := router.Group(basePath)
	registerRoutes(v1)
	if swag.GetSwagger("swagger") == nil {
		logrus.Warn("Swagger spec is not loaded! Возможно, пакет docs не подключен.")
	}
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	err := router.Run(":8082")
	if err != nil {
		log.Error(err)
	}
}
