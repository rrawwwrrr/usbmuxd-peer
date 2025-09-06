package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios/imagemounter"
	"github.com/danielpaulus/go-ios/ios/mobileactivation"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/mcinstall"
	"github.com/danielpaulus/go-ios/ios/simlocation"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Активация устройства. Устройства необходимо активировать и связаться с серверами Apple перед использованием.
// Info                godoc
// @Summary      Активировать устройство по UDID
// @Description  Возвращает ошибку, если активация не удалась. В противном случае {"message":"Активация успешна"}
// @Tags         general_device_specific
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/activate [post]
func Activate(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	err := mobileactivation.Activate(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}
	c.IndentedJSON(http.StatusOK, GenericResponse{Message: "Activation successful"})
}

func GetImages(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	conn, err := imagemounter.NewImageMounter(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	signatures, err := conn.ListImages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}

	res := make([]string, len(signatures))
	for i, sig := range signatures {
		res[i] = fmt.Sprintf("%x", sig)
	}
	c.JSON(http.StatusOK, res)

}
func InstallImage(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	auto := c.Query("auto")
	if auto == "true" {
		basedir := c.Query("basedir")
		if basedir == "" {
			basedir = "./devimages"
		}

		path, err := imagemounter.DownloadImageFor(device, basedir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			return
		}
		err = imagemounter.MountImage(device, path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, "ok")
		return
	}
	body := c.Request.Body
	defer body.Close()

	tempfile, err := os.CreateTemp(os.TempDir(), "go-ios")
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	tempfilepath := tempfile.Name()
	defer os.Remove(tempfilepath)
	_, err = io.Copy(tempfile, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	err = tempfile.Close()
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	err = imagemounter.MountImage(device, tempfilepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, "ok")
	return
}

// Получение информации об устройстве
// Info                godoc
// @Summary      Получить информацию о блокировке устройства по UDID
// @Description  Возвращает все значения lockdown и дополнительные свойства instruments для устройств с включенной разработкой.
// @Tags         general_device_specific
// @Produce      json
// @Param        udid  path      string  true  "UDID устройства"
// @Success      200  {object}  map[string]interface{}
// @Router       /device/{udid}/info [get]
func Info(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	allValues, err := ios.GetValuesPlist(device)
	if err != nil {
		print(err)
	}
	svc, err := instruments.NewDeviceInfoService(device)
	if err != nil {
		log.Debugf("could not open instruments, probably dev image not mounted %v", err)
	}
	if err == nil {
		info, err := svc.NetworkInformation()
		if err != nil {
			log.Debugf("error getting networkinfo from instruments %v", err)
		} else {
			allValues["instruments:networkInformation"] = info
		}
		info, err = svc.HardwareInformation()
		if err != nil {
			log.Debugf("error getting hardwareinfo from instruments %v", err)
		} else {
			allValues["instruments:hardwareInformation"] = info
		}
	}
	c.IndentedJSON(http.StatusOK, allValues)
}

// Скриншот с устройства
// Screenshot                godoc
// @Summary      Получить скриншот устройства
// @Description Делает скриншот в формате PNG и возвращает его.
// @Tags         general_device_specific
// @Produce      png
// @Param        udid  path      string  true  "UDID устройства"
// @Success      200  {object}  []byte
// @Router       /device/{udid}/screenshot [get]
func Screenshot(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	screenshotService, err := instruments.NewScreenshotService(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}
	defer screenshotService.Close()

	imageBytes, err := screenshotService.TakeScreenshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Data(http.StatusOK, "application/octet-stream", imageBytes)
}

// Изменение текущего местоположения устройства
// @Summary      Изменить текущее местоположение устройства
// @Description Изменяет текущее местоположение устройства на указанные широту и долготу
// @Tags         general_device_specific
// @Produce      json
// @Param        latitude  query      string  true  "Широта местоположения"
// @Param        longtitude  query      string  true  "Долгота местоположения"
// @Success      200  {object}  GenericResponse
// @Failure      422  {object}  GenericResponse
// @Failure      500  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/setlocation [post]
func SetLocation(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	latitude := c.Query("latitude")
	if latitude == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "latitude query param is missing"})
		return
	}

	longtitude := c.Query("longtitude")
	if longtitude == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "longtitude query param is missing"})
		return
	}

	err := simlocation.SetLocation(device, latitude, longtitude)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "Device location set to latitude=" + latitude + ", longtitude=" + longtitude})
}

// Сброс местоположения устройства к реальному
// @Summary      Сброс изменённого местоположения устройства
// @Description  Сбрасывает изменённое местоположение устройства к реальному
// @Tags         general_device_specific
// @Produce      json
// @Success      200
// @Failure      500  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/resetlocation [post]
func ResetLocation(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	err := simlocation.ResetLocation(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "Device location reset"})
}

// Получение списка установленных профилей
// @Summary      Получить список профилей
// @Description  Получить список установленных профилей на iOS-устройстве
// @Tags         general_device_specific
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  GenericResponse
// @Failure      404  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/profiles [get]
func GetProfiles(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	mcinstallconn, err := mcinstall.New(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed getting device list with error", "error": err.Error()})
		return
	}

	defer mcinstallconn.Close()

	profileInfo, err := mcinstallconn.HandleList()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Failed getting profile list with error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, profileInfo)
}

//========================================
// DEVICE STATE CONDITIONS
//========================================

var (
	deviceConditionsMap   = make(map[string]deviceCondition)
	deviceConditionsMutex sync.Mutex
)

type deviceCondition struct {
	ProfileType  instruments.ProfileType
	Profile      instruments.Profile
	StateControl *instruments.DeviceStateControl
}

// Получение списка доступных условий для устройства
// @Summary      Получить список доступных условий устройства
// @Description  Получить список условий, которые можно применить к устройству
// @Tags         general_device_specific
// @Produce      json
// @Success      200  {object}  []instruments.ProfileType
// @Failure      500  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/conditions [get]
func GetSupportedConditions(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	control, err := instruments.NewDeviceStateControl(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	profileTypes, err := control.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, profileTypes)
}

// Включение условия на устройстве
// @Summary      Включить условие на устройстве
// @Description  Включает условие на устройстве по указанным profileTypeID и profileID
// @Tags         general_device_specific
// @Produce      json
// @Param        udid path string true "UDID устройства"
// @Param        profileTypeID  query      string  true  "Идентификатор типа профиля, например SlowNetworkCondition"
// @Param        profileID  query      string  true  "Идентификатор под-профиля, например SlowNetwork100PctLoss"
// @Success      200  {object}  GenericResponse
// @Failure      500  {object}  GenericResponse
// @Router       /device/{udid}/conditions [put]
func EnableDeviceCondition(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	udid := device.Properties.SerialNumber

	deviceConditionsMutex.Lock()
	defer deviceConditionsMutex.Unlock()

	conditionedDevice, exists := deviceConditionsMap[udid]
	if exists {
		c.JSON(http.StatusOK, GenericResponse{Error: "Device has an active condition - profileTypeID=" + conditionedDevice.ProfileType.Identifier + ", profileID=" + conditionedDevice.Profile.Identifier})
		return
	}

	profileTypeID := c.Query("profileTypeID")
	if profileTypeID == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "profileTypeID query param is missing"})
		return
	}

	profileID := c.Query("profileID")
	if profileID == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "profileID query param is missing"})
		return
	}

	control, err := instruments.NewDeviceStateControl(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	profileTypes, err := control.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	profileType, profile, err := instruments.VerifyProfileAndType(profileTypes, profileTypeID, profileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	err = control.Enable(profileType, profile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	// Когда мы применяем условие с использованием конкретного указателя *instruments.DeviceStateControl,
	// для его отключения нужен тот же самый указатель.
	// Создание нового *DeviceStateControl и использование того же profileType **не отключит** уже активное условие.
	// По этой причине мы храним карту `deviceConditions`, которая содержит оригинальные указатели *DeviceStateControl,
	// которые можно использовать в `DisableDeviceCondition()` для успешного отключения активного условия.
	newDeviceCondition := deviceCondition{ProfileType: profileType, Profile: profile, StateControl: control}
	deviceConditionsMap[device.Properties.SerialNumber] = newDeviceCondition

	c.JSON(http.StatusOK, GenericResponse{Message: "Enabled condition for ProfileType=" + profileTypeID + " and Profile=" + profileID})
}

// Отключение текущего активного условия на устройстве
// @Summary      Отключить текущее активное условие на устройстве
// @Description  Отключает текущее активное условие на устройстве
// @Tags         general_device_specific
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      500  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Router       /device/{udid}/conditions [post]
func DisableDeviceCondition(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	udid := device.Properties.SerialNumber

	deviceConditionsMutex.Lock()
	defer deviceConditionsMutex.Unlock()

	conditionedDevice, exists := deviceConditionsMap[udid]
	if !exists {
		c.JSON(http.StatusOK, GenericResponse{Error: "Device has no active condition"})
		return
	}

	// Disable() does not throw an error if the respective condition is not active on the device
	err := conditionedDevice.StateControl.Disable(conditionedDevice.ProfileType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	delete(deviceConditionsMap, udid)

	c.JSON(http.StatusOK, GenericResponse{Message: "Device condition disabled"})
}

// ========================================
// DEVICE PAIRING
// ========================================
// Pair устройства
// @Summary      Сопряжение устройства с/без режима Supervised
// @Description  Сопряжение устройства с/без режима Supervised
// @Tags         general_device_specific
// @Produce      json
// @Success      200  {object}  GenericResponse
// @Failure      500  {object}  GenericResponse
// @Failure      422  {object}  GenericResponse
// @Param        udid path string true "UDID устройства"
// @Param        supervised query string true "Установить, находится ли устройство в режиме Supervised - true/false"
// @Param        p12file formData file false "Файл Supervision *.p12"
// @Param        supervision_password formData string false "Пароль для Supervision"
// @Router       /device/{udid}/pair [post]
func PairDevice(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	supervised := c.Query("supervised")
	if supervised == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "supervised query param is missing (true/false)"})
		return
	}

	if supervised == "false" {
		err := ios.Pair(device)
		if err != nil {
			c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, GenericResponse{Message: "Device paired"})
		return
	}

	file, _, err := c.Request.FormFile("p12file")
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "Could not parse p12 file from form-data or no file provided, err:" + err.Error()})
		return
	}
	p12fileBuf := new(bytes.Buffer)
	p12fileBuf.ReadFrom(file)

	supervision_password := c.Request.Header.Get("Supervision-Password")
	if supervision_password == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "you must provide non-empty `Supervision-Password` header with the request"})
		return
	}

	err = ios.PairSupervised(device, p12fileBuf.Bytes(), supervision_password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "Device paired"})
}

func GetInfoFirstDevice() DeviceInfo {
	const maxAttempts = 10

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		devices, err := ios.ListDevices()
		if err != nil || len(devices.DeviceList) == 0 {
			log.Infof("Failed to list devices (attempt %d/%d)", attempt, maxAttempts)
			time.Sleep(time.Second)
			continue
		}

		device := devices.DeviceList[0]
		allValues, err := ios.GetValuesPlist(device)
		if err != nil {
			log.Infof("Failed to get device values: %v (attempt %d/%d)", err, attempt, maxAttempts)
			time.Sleep(time.Second)
			continue
		}

		svc, err := instruments.NewDeviceInfoService(device)
		if err != nil {
			log.Debugf("could not open instruments, probably dev image not mounted %v", err)
		} else {
			if info, err := svc.NetworkInformation(); err == nil {
				allValues["instruments:networkInformation"] = info
			} else {
				log.Debugf("error getting networkinfo from instruments %v", err)
			}

			if info, err := svc.HardwareInformation(); err == nil {
				allValues["instruments:hardwareInformation"] = info
			} else {
				log.Debugf("error getting hardwareinfo from instruments %v", err)
			}
		}

		var deviceInfo DeviceInfo
		err = fillStructFromMap(allValues, &deviceInfo)
		log.Info(allValues)
		if err != nil {
			log.WithError(err).Infof("Failed to fill device info (attempt %d/%d)", attempt, maxAttempts)
			time.Sleep(time.Second)
			continue
		}
		return deviceInfo
	}

	log.Fatal("Failed to get device info after 10 attempts, exiting application")
	return DeviceInfo{}
}

func fillStructFromMap(allValues map[string]interface{}, out interface{}) error {
	// Сначала преобразуем map в JSON
	data, err := json.Marshal(allValues)
	if err != nil {
		return err
	}
	// Потом распакуем JSON в структуру
	return json.Unmarshal(data, out)
}
