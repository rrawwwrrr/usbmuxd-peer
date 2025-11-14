package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type InstallAppByUrlRequest struct {
	URL string `json:"url" binding:"required,url"`
}

const MaxFileSize = 200 * 1024 * 1024 // 200 MB

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

// InstallAppFromFileController
// @Summary      Установить приложение из загруженного файла
// @Description  Загружает IPA-файл и устанавливает его на устройство
// @Tags         apps
// @Accept       multipart/form-data
// @Produce      json
// @Param        udid  path   string  true  "UDID устройства"
// @Param        file  formData file   true  "IPA-файл (.ipa)"
// @Success      200   {object} GenericResponse
// @Failure      400   {object} GenericResponse
// @Failure      413   {object} GenericResponse
// @Failure      500   {object} GenericResponse
// @Router       /device/{udid}/apps/install [post]
func InstallAppFromFile(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: "missing 'file' in form data"})
		return
	}

	if err := validateFileSize(file.Size); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: err.Error()})
		return
	}

	dst := filepath.Join(os.TempDir(), uuid.New().String()+".ipa")
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "failed to save uploaded file"})
		return
	}
	defer CleanupTempFile(dst)

	if err := installApp(device, dst); err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "installation failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "App installed successfully"})
}

// InstallAppByUrlController
// @Summary      Установить приложение по ссылке
// @Description  Скачивает IPA по URL и устанавливает на устройство
// @Tags         apps
// @Accept       json
// @Produce      json
// @Param        udid  path   string            true  "UDID устройства"
// @Param        req   body   InstallAppByUrlRequest true "URL на IPA-файл"
// @Success      200   {object} GenericResponse
// @Failure      400   {object} GenericResponse
// @Failure      413   {object} GenericResponse
// @Failure      500   {object} GenericResponse
// @Router       /device/{udid}/apps/install-by-url [post]
func InstallAppByUrl(c *gin.Context) {
	device := c.MustGet(IOS_KEY).(ios.DeviceEntry)

	var req InstallAppByUrlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	data, err := downloadFileByURL(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{Error: "failed to download file: " + err.Error()})
		return
	}

	dst, err := saveTempFile(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "failed to save downloaded file"})
		return
	}
	defer CleanupTempFile(dst)

	if err := installApp(device, dst); err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{Error: "installation failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{Message: "App installed successfully"})
}

func CleanupTempFile(path string) {
	if err := os.Remove(path); err != nil {
		log.Printf("Warning: failed to delete temp file %s: %v", path, err)
	}
}
func saveTempFile(data []byte) (string, error) {
	appDownloadFolder := os.Getenv("APP_DOWNLOAD_FOLDER")
	if appDownloadFolder == "" {
		appDownloadFolder = os.TempDir()
	}

	dst := filepath.Join(appDownloadFolder, uuid.New().String()+".ipa")
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", err
	}

	return dst, nil
}

// validateFileSize — проверяет размер файла
func validateFileSize(size int64) error {
	if size == 0 {
		return fmt.Errorf("uploaded file is empty")
	}
	if size > MaxFileSize {
		return fmt.Errorf("file size exceeds %d bytes", MaxFileSize)
	}
	return nil
}

// downloadFileByURL — скачивает файл по URL и возвращает его содержимое
func downloadFileByURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, MaxFileSize+1)
	buf, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}

	if len(buf) > MaxFileSize {
		return nil, fmt.Errorf("file size exceeds %d bytes", MaxFileSize)
	}

	return buf, nil
}

func installApp(device ios.DeviceEntry, ipaPath string) error {
	conn, err := zipconduit.New(device)
	if err != nil {
		return fmt.Errorf("failed to create ZipConduit connection: %w", err)
	}

	err = conn.SendFile(ipaPath)
	if err != nil {
		return fmt.Errorf("failed to send file via ZipConduit: %w", err)
	}
	return nil
}
