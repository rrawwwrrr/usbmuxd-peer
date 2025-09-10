package api

import (
	"github.com/gin-gonic/gin"
)

func Download(c *gin.Context) {
	filePath := "/files/ddi-15F31d.zip" // Указываем путь к файлу

	c.File(filePath)
}
