package main

import (
	"goios-peer/goios"

	log "github.com/sirupsen/logrus"
)

func main() {
	customFormatter := new(log.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	log.SetFormatter(customFormatter)
	log.Info("Hello Walrus before FullTimestamp=true")
	customFormatter.FullTimestamp = true
	log.Info("Hello Walrus after FullTimestamp=true")
	//log.SetLevel(log.TraceLevel)
	goios.Start()
	//rest.StartRestAPI()

}
