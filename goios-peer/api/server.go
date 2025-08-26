package api

import (
	"io"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"

	_ "goios-peer/docs"
)

func StartRestAPI() {
	router := gin.Default()
	log := logrus.New()
	myfile, _ := os.Create("go-ios.log")
	gin.DefaultWriter = io.MultiWriter(myfile, os.Stdout)
	TunnelStart()
	router.Use(MyLogger(log), gin.Recovery())

	v1 := router.Group("/api/v1")
	registerRoutes(v1)
	if swag.GetSwagger("swagger") == nil {
		logrus.Warn("Swagger spec is not loaded! Возможно, пакет docs не подключен.")
	}
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	err := router.Run(":8082")
	if err != nil {
		log.Error(err)
	}
}
