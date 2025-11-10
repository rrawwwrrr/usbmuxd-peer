package api

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type InstallAppRequest struct {
	URL string `json:"url" binding:"required,url"`
}

// Список приложений на устройстве
// @Summary      Список приложений на устройстве
// @Description  Получить список установленных приложений на устройстве
// @Tags         apps
// @Produce      json
// @Param        udid path string true "UDID устройства"
// @Success      200 {object} []installationproxy.AppInfo
// @Failure      500 {object} GenericResponse
// @Router       /device/{udid}/apps [get]
func ListApps(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	svc, _ := installationproxy.New(device)
	var err error
	var response []installationproxy.AppInfo
	response, err = svc.BrowseAllApps()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
	}
	c.IndentedJSON(http.StatusOK, response)
}

// Запуск приложения на устройстве
// @Summary      Запуск приложения на устройстве
// @Description  Запустить приложение на устройстве по указанному bundleID
// @Tags         apps
// @Produce      json
// @Param        bundleID query string true "идентификатор bundle целевого приложения"
// @Param        udid path string true "UDID устройства"
// @Success      200  {object} GenericResponse
// @Failure      500  {object} GenericResponse
// @Router       /device/{udid}/apps/launch [post]
func LaunchApp(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	bundleID := c.Query("bundleID")
	if bundleID == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "bundleID query param is missing"})
		return
	}

	pControl, err := instruments.NewProcessControl(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	_, err = pControl.LaunchApp(bundleID, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: bundleID + " launched successfully"})
}

// Завершение работы приложения на устройстве
// @Summary      Завершение работы приложения на устройстве
// @Description  Завершить работу приложения на устройстве по указанному bundleID
// @Tags         apps
// @Produce      json
// @Param        bundleID query string true "идентификатор bundle целевого приложения"
// @Param        udid path string true "UDID устройства"
// @Success      200 {object} GenericResponse
// @Failure      500 {object} GenericResponse
// @Router       /device/{udid}/apps/kill [post]
func KillApp(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)
	processName := ""

	bundleID := c.Query("bundleID")
	if bundleID == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "bundleID query param is missing"})
		return
	}

	pControl, err := instruments.NewProcessControl(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	svc, err := installationproxy.New(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	response, err := svc.BrowseAllApps()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	for _, app := range response {
		if app.CFBundleIdentifier() == bundleID {
			processName = app.CFBundleExecutable()
			break
		}
	}

	if processName == "" {
		c.JSON(http.StatusNotFound, GenericResponse{Message: bundleID + " is not installed"})
		return
	}

	service, err := instruments.NewDeviceInfoService(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}
	defer service.Close()

	processList, err := service.ProcessList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	for _, p := range processList {
		if p.Name == processName {
			err = pControl.KillProcess(p.Pid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
				return
			}
			c.JSON(http.StatusOK, GenericResponse{Message: bundleID + " successfully killed"})
			return
		}
	}

	c.JSON(http.StatusOK, GenericResponse{Message: bundleID + " is not running"})
}

// Установка приложения на устройстве
// @Summary      Установка приложения на устройстве
// @Description  Установить приложение на устройстве, загрузив ipa-файл напрямую или по ссылке
// @Tags         apps
// @Produce      json
// @Param        file formData file false "ipa-файл для установки"
// @Param        url  body    InstallAppRequest false "URL для скачивания ipa-файла"
// @Param        udid path    string true "UDID устройства"
// @Success      200 {object} GenericResponse
// @Failure      400 {object} GenericResponse
// @Failure      413 {object} GenericResponse
// @Failure      500 {object} GenericResponse
// @Router       /device/{udid}/apps/install [post]
func InstallApp(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	var dst string
	var cleanup func() // отложенная очистка временного файла
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	// Определяем, передан ли файл через multipart/form-data
	_, fileHeader, _ := c.Request.FormFile("file")
	hasFile := fileHeader != nil

	// Определяем, передано ли тело как JSON с URL
	var jsonReq InstallAppRequest
	hasJSON := false
	if c.GetHeader("Content-Type") == "application/json" {
		if err := c.ShouldBindJSON(&jsonReq); err == nil && jsonReq.URL != "" {
			hasJSON = true
		}
	}

	// Валидация: должен быть либо файл, либо URL — но не оба и не ни один
	if hasFile && hasJSON {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: "provide either 'file' or 'url', not both"})
		return
	}
	if !hasFile && !hasJSON {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: "either 'file' (formData) or 'url' (JSON) is required"})
		return
	}

	appDownloadFolder := os.Getenv("APP_DOWNLOAD_FOLDER")
	if appDownloadFolder == "" {
		appDownloadFolder = os.TempDir()
	}

	if hasFile {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, GenericResponse{Error: "file form-data is missing"})
			return
		}

		if file.Size == 0 {
			c.JSON(http.StatusBadRequest, GenericResponse{Error: "uploaded file is empty"})
			return
		}
		if file.Size > 200*1024*1024 {
			c.JSON(http.StatusRequestEntityTooLarge, GenericResponse{Error: "file size exceeds the 200MB limit"})
			return
		}

		dst = path.Join(appDownloadFolder, uuid.New().String()+".ipa")
		if err := c.SaveUploadedFile(file, dst); err != nil {
			c.JSON(http.StatusInternalServerError, GenericResponse{Error: "failed to save uploaded file"})
			return
		}

		cleanup = func() {
			if err := os.Remove(dst); err != nil {
				log.Printf("Warning: failed to remove temp file %s: %v", dst, err)
			}
		}

	} else if hasJSON {
		// Скачиваем файл по URL
		resp, err := http.Get(jsonReq.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, GenericResponse{Error: "failed to fetch file from URL: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadRequest, GenericResponse{Error: fmt.Sprintf("failed to download file: HTTP %d", resp.StatusCode)})
			return
		}

		// Ограничиваем размер — читаем с лимитом
		limitReader := io.LimitReader(resp.Body, 200*1024*1024+1) // +1 для детекции превышения
		var buf bytes.Buffer
		n, err := buf.ReadFrom(limitReader)
		if err != nil {
			c.JSON(http.StatusInternalServerError, GenericResponse{Error: "error reading downloaded file"})
			return
		}

		if n > 200*1024*1024 {
			c.JSON(http.StatusRequestEntityTooLarge, GenericResponse{Error: "downloaded file exceeds the 200MB limit"})
			return
		}

		dst = path.Join(appDownloadFolder, uuid.New().String()+".ipa")
		if err := os.WriteFile(dst, buf.Bytes(), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, GenericResponse{Error: "failed to save downloaded file"})
			return
		}

		cleanup = func() {
			if err := os.Remove(dst); err != nil {
				log.Printf("Warning: failed to remove temp file %s: %v", dst, err)
			}
		}
	}

	// Установка приложения через ZipConduit
	conn, err := zipconduit.New(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "Unable to setup ZipConduit connection"})
		return
	}

	if err := conn.SendFile(dst); err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "Unable to install uploaded app: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "App installed successfully"})
}

// Удаление приложения с устройства
// @Summary      Удаление приложения с устройства
// @Description  Удалить приложение с устройства по указанному bundleID
// @Tags         apps
// @Produce      json
// @Param        bundleID query string true "bundleID приложения"
// @Param        udid path string true "UDID устройства"
// @Success      200 {object} GenericResponse
// @Failure      500 {object} GenericResponse
// @Router       /device/{udid}/apps/uninstall [delete]
func UninstallApp(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	bundleID := c.Query("bundleID")
	if bundleID == "" {
		c.JSON(http.StatusUnprocessableEntity, GenericResponse{Error: "bundleID query param is missing"})
		return
	}

	svc, err := installationproxy.New(device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}
	defer svc.Close()

	err = svc.Uninstall(bundleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: bundleID + " uninstalled successfully"})
}
