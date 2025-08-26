package main

import (
	"goios-peer/api"
	"goios-peer/socket"

	log "github.com/sirupsen/logrus"
)

func main() {
	customFormatter := new(log.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	log.SetFormatter(customFormatter)
	customFormatter.FullTimestamp = true
	log.SetLevel(log.DebugLevel)
	//goios.Start()
	go socket.Start()
	api.StartRestAPI()

}
