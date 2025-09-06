package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type State struct {
	Device map[string]interface{} `json:"info"`
	Wda    WdaStatus              `json:"wda"`
}

type WdaStatus struct {
	Status    string `json:"status"`
	SessionId string `json:"sessionId"`
	Detail    string `json:"detail"`
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

var (
	clients   = make(map[*websocket.Conn]bool) // Активные подключения
	broadcast = make(chan []byte)              // Канал рассылки
	state     = &State{
		Device: GetInfoFirstDevice(),
		Wda:    WdaStatus{},
	}
	stateMu  sync.RWMutex
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func serveWs(w http.ResponseWriter, r *http.Request) {
	log.Info("Websocket upgrader...")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("Ошибка при апгрейде соединения:", err)
		return
	}
	defer conn.Close()

	clients[conn] = true
	log.Infof("Websocket подключен: %v (текущее количество клиентов: %d)", conn.RemoteAddr(), len(clients))

	stateMu.RLock()
	fullState := *state
	stateMu.RUnlock()
	SendWSMessageToConn(conn, "state", fullState)

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Warnf("Клиент отключился: %v, причина: %v", conn.RemoteAddr(), err)
			delete(clients, conn)
			log.Infof("Клиент удалён: %v (текущее количество клиентов: %d)", conn.RemoteAddr(), len(clients))
			break
		}
		broadcast <- p
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		log.Infof("handleMessages: получено сообщение для рассылки, размер: %d байт, клиентов: %d", len(msg), len(clients))
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Errorf("Ошибка при отправке сообщения клиенту %v: %v", client.RemoteAddr(), err)
				client.Close()
				delete(clients, client)
				log.Infof("Клиент удалён при ошибке отправки: %v (текущее количество клиентов: %d)", client.RemoteAddr(), len(clients))
			}
		}
	}
}

func SendWSMessage(msgType string, data interface{}) {
	log.Info("SendWSMessage start")

	msg := WSMessage{
		Type: msgType,
		Data: data,
	}
	log.Info(msg)
	b, _ := json.Marshal(msg)
	broadcast <- b

	log.Info("SendWSMessage stop")
}

func SendWSMessageToConn(conn *websocket.Conn, msgType string, data interface{}) {
	msg := WSMessage{
		Type: msgType,
		Data: data,
	}
	b, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, b)
}

func UpdateWdaStatus(udid, status, sessionId, detail string) {
	log.Info("Start update WDA")
	stateMu.Lock()
	state.Wda = WdaStatus{
		Status:    status,
		SessionId: sessionId,
		Detail:    detail,
	}
	updated := state.Wda
	stateMu.Unlock()
	log.Info("Stop update WDA")
	log.Info("Start send ws WDA")
	SendWSMessage("wda_status", map[string]interface{}{"udid": udid, "status": updated})
	log.Info("Stop send ws WDA")
}

//func UpdateDeviceInfo(info DeviceInfo) {
//	stateMu.Lock()
//	state.Device = info
//	stateMu.Unlock()
//	SendWSMessage("device_info", info)
//}
