package model

type DeviceInfo struct {
	UniqueDeviceID string `json:"UniqueDeviceID"`
	SerialNumber   string `json:"SerialNumber"`
	ProductVersion string `json:"ProductVersion"`
	ProductName    string `json:"ProductName"`
	ProductType    string `json:"ProductType"`
	DeviceName     string `json:"DeviceName"`
	ModelNumber    string `json:"ModelNumber"`
}
