package tools

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

func ConvertToJSONString(data interface{}) string {
	b, err := json.Marshal(data)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return string(b)
}

func ExitIfError(msg string, err error) {
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatalf(msg)
	}
}
