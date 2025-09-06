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

type DeviceInfo struct {
	ActivationState                       string            `json:"ActivationState"`
	ActivationStateAcknowledged           string            `json:"ActivationStateAcknowledged"`
	BasebandActivationTicketVersion       string            `json:"BasebandActivationTicketVersion"`
	BasebandCertId                        string            `json:"BasebandCertId"`
	BasebandChipID                        string            `json:"BasebandChipID"`
	BasebandKeyHashInformation            map[string]string `json:"BasebandKeyHashInformation"`
	BasebandMasterKeyHash                 string            `json:"BasebandMasterKeyHash"`
	BasebandRegionSKU                     string            `json:"BasebandRegionSKU"`
	BasebandSerialNumber                  string            `json:"BasebandSerialNumber"`
	BasebandStatus                        string            `json:"BasebandStatus"`
	BasebandVersion                       string            `json:"BasebandVersion"`
	BluetoothAddress                      string            `json:"BluetoothAddress"`
	BoardId                               string            `json:"BoardId"`
	BootSessionID                         string            `json:"BootSessionID"`
	BrickState                            string            `json:"BrickState"`
	BuildVersion                          string            `json:"BuildVersion"`
	CPUArchitecture                       string            `json:"CPUArchitecture"`
	CarrierBundleInfoArray                []interface{}     `json:"CarrierBundleInfoArray"`
	CertID                                string            `json:"CertID"`
	ChipID                                string            `json:"ChipID"`
	ChipSerialNo                          string            `json:"ChipSerialNo"`
	DeviceClass                           string            `json:"DeviceClass"`
	DeviceColor                           string            `json:"DeviceColor"`
	DeviceName                            string            `json:"DeviceName"`
	DieID                                 string            `json:"DieID"`
	EthernetAddress                       string            `json:"EthernetAddress"`
	FirmwareVersion                       string            `json:"FirmwareVersion"`
	FusingStatus                          string            `json:"FusingStatus"`
	HardwareModel                         string            `json:"HardwareModel"`
	HardwarePlatform                      string            `json:"HardwarePlatform"`
	HasSiDP                               string            `json:"HasSiDP"`
	HostAttached                          string            `json:"HostAttached"`
	HumanReadableProductVersionString     string            `json:"HumanReadableProductVersionString"`
	InternationalMobileEquipmentIdentity  string            `json:"InternationalMobileEquipmentIdentity"`
	InternationalMobileEquipmentIdentity2 string            `json:"InternationalMobileEquipmentIdentity2"`
	MLBSerialNumber                       string            `json:"MLBSerialNumber"`
	MobileEquipmentIdentifier             string            `json:"MobileEquipmentIdentifier"`
	MobileSubscriberCountryCode           string            `json:"MobileSubscriberCountryCode"`
	MobileSubscriberNetworkCode           string            `json:"MobileSubscriberNetworkCode"`
	ModelNumber                           string            `json:"ModelNumber"`
	NonVolatileRAM                        map[string]string `json:"NonVolatileRAM"`
	PairRecordProtectionClass             string            `json:"PairRecordProtectionClass"`
	PartitionType                         string            `json:"PartitionType"`
	PasswordProtected                     string            `json:"PasswordProtected"`
	PkHash                                string            `json:"PkHash"`
	ProductName                           string            `json:"ProductName"`
	ProductType                           string            `json:"ProductType"`
	ProductVersion                        string            `json:"ProductVersion"`
	ProductionSOC                         string            `json:"ProductionSOC"`
	ProtocolVersion                       string            `json:"ProtocolVersion"`
	ProximitySensorCalibration            string            `json:"ProximitySensorCalibration"`
	RegionInfo                            string            `json:"RegionInfo"`
	SIMStatus                             string            `json:"SIMStatus"`
	SIMTrayStatus                         string            `json:"SIMTrayStatus"`
	SerialNumber                          string            `json:"SerialNumber"`
	SoftwareBehavior                      string            `json:"SoftwareBehavior"`
	SoftwareBundleVersion                 string            `json:"SoftwareBundleVersion"`
	SupportedDeviceFamilies               []int             `json:"SupportedDeviceFamilies"`
	TelephonyCapability                   string            `json:"TelephonyCapability"`
	TimeIntervalSince1970                 string            `json:"TimeIntervalSince1970"`
	TimeZone                              string            `json:"TimeZone"`
	TimeZoneOffsetFromUTC                 string            `json:"TimeZoneOffsetFromUTC"`
	TrustedHostAttached                   string            `json:"TrustedHostAttached"`
	UniqueChipID                          string            `json:"UniqueChipID"`
	UniqueDeviceID                        string            `json:"UniqueDeviceID"`
	UseRaptorCerts                        string            `json:"UseRaptorCerts"`
	Uses24HourClock                       string            `json:"Uses24HourClock"`
	WiFiAddress                           string            `json:"WiFiAddress"`
	WirelessBoardSerialNumber             string            `json:"WirelessBoardSerialNumber"`
	KCTPostponementStatus                 string            `json:"kCTPostponementStatus"`
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

	stateMu.RLock()
	fullState := *state
	stateMu.RUnlock()
	SendWSMessageToConn(conn, "state", fullState)

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Error("Ошибка при чтении сообщения:", err)
			delete(clients, conn)
			break
		}
		broadcast <- p
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Error("Ошибка при отправке сообщения:", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func SendWSMessage(msgType string, data interface{}) {
	msg := WSMessage{
		Type: msgType,
		Data: data,
	}
	b, _ := json.Marshal(msg)
	broadcast <- b
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
	stateMu.Lock()
	state.Wda = WdaStatus{
		Status:    status,
		SessionId: sessionId,
		Detail:    detail,
	}
	updated := state.Wda
	stateMu.Unlock()
	SendWSMessage("wda_status", map[string]interface{}{"udid": udid, "status": updated})
}

//func UpdateDeviceInfo(info DeviceInfo) {
//	stateMu.Lock()
//	state.Device = info
//	stateMu.Unlock()
//	SendWSMessage("device_info", info)
//}
