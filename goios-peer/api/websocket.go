package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var clients = make(map[*websocket.Conn]bool) // Словарь для хранения активных подключений
var broadcast = make(chan []byte)            // Канал для рассылки сообщений
var lastStatuses = make(map[string][]byte)   // udid -> json-статус
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("Ошибка при обновлении соединения:", err)
		return
	}
	defer conn.Close()

	clients[conn] = true // Добавляем подключение в список активных

	// 1. Отправляем информацию о первом устройстве
	if info := GetInfoFirstDevice(); info != nil {
		msg := map[string]interface{}{
			"type": "device_info_first",
			"data": info,
		}
		data, _ := json.Marshal(msg)
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Error("Ошибка при отправке device_info_first:", err)
			conn.Close()
			delete(clients, conn)
			return
		}
	}

	// 2. Отправляем все последние статусы
	for _, status := range lastStatuses {
		err := conn.WriteMessage(websocket.TextMessage, status)
		if err != nil {
			log.Error("Ошибка при отправке статуса новому клиенту:", err)
			conn.Close()
			delete(clients, conn)
			return
		}
	}

	// 3. Основной цикл чтения
	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Error("Ошибка при чтении сообщения:", err)
			delete(clients, conn) // Удаляем подключение из списка при ошибке чтения
			break
		}

		broadcast <- p // Отправляем полученное сообщение в канал для рассылки
	}
}

func handleMessages() {
	for {
		msg := <-broadcast // Получаем сообщение из канала рассылки

		// Отправляем сообщение каждому активному подключению
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Error("Ошибка при отправке сообщения:", err)
				client.Close()
				delete(clients, client) // Удаляем подключение из списка при ошибке отправки
			}
		}
	}
}

func SendWdaStatus(status string, udid string, sessionId string, detail string) {
	msg := map[string]interface{}{
		"type":      "wda_status",
		"status":    status,
		"udid":      udid,
		"sessionId": sessionId,
		"detail":    detail,
	}
	data, _ := json.Marshal(msg)
	lastStatuses[udid] = data
	broadcast <- data
}
