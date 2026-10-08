package controllers

import (
	"bytes"
	"context"
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ResetSystemRequest struct {
	Confirm string `json:"confirm" binding:"required" enums:"RESET"`
}

// ResetSystem 恢复出厂设置 - 清空所有业务数据
// @Summary 系统重置
// @Description 系统重置
// @Tags System
// @Accept json
// @Produce json
// @Param body body ResetSystemRequest true "固定传 confirm=RESET"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
func ResetSystem(c *gin.Context) {
	// 这里可以加一个简单的校验，或者从 Header 传一个 RESET 标记
	var req ResetSystemRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请正确输入确认码"})
		return
	}

	// 开启事务执行清空
	tx := config.DB.Begin()

	// 清空所有业务表 (按照依赖关系逆序删除或直接 TRUNCATE)
	// 注意：MySQL 中如果有外键约束，TRUNCATE 可能会失败，可以使用 DELETE
	tables := []interface{}{
		&models.AIProviderConfig{},
		&models.AIImport{},
		&models.AIAgentSession{},
		&models.AIAgentMessage{},
		&models.AIAgentAction{},
		&models.FinancialLog{},
		&models.InventoryLog{},
		&models.TransferRecord{},
		&models.StocktakeRecord{},
		&models.InboundRecord{},
		&models.Expense{},
		&models.OrderItem{},
		&models.Order{},
		&models.Product{},
		&models.Category{},
		&models.MonthlyAttendance{},
		&models.Employee{},
		&models.Account{},
		&models.AdminUser{},
	}

	for _, table := range tables {
		if err := tx.Unscoped().Where("1 = 1").Delete(table).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "系统重置失败: " + err.Error()})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "系统已成功恢复出厂设置"})
}

// ExportBackup 导出数据库备份 (.sql)
// @Summary 导出数据备份
// @Description 导出数据备份
// @Tags System
// @Produce application/octet-stream
// @Success 200 {file} binary "SQL 备份文件"
// @Security ApiKeyAuth
// @ID get_api_system_backup
// @Router /api/system/backup [get]
func ExportBackup(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	tempFile, err := os.CreateTemp("", "erpwonchon-backup-*.sql")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建备份临时文件失败: " + err.Error()})
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	args := mysqlClientArgs()
	cmd := exec.CommandContext(ctx, "mysqldump", args...)
	var stderr bytes.Buffer
	cmd.Stdout = tempFile
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "数据库备份执行失败: " + stderr.String()})
		return
	}

	if err := tempFile.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "备份文件写入失败: " + err.Error()})
		return
	}

	filename := fmt.Sprintf("gubuy_erp_backup_%s.sql", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/sql")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(tempPath, filename)
}

// ImportBackup 导入数据库备份 (覆盖式)
// @Summary 导入数据备份
// @Description 导入数据备份
// @Tags System
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "SQL 备份文件"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_system_restore
// @Router /api/system/restore [post]
func ImportBackup(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未接收到上传的文件"})
		return
	}

	if filepath.Ext(file.Filename) != ".sql" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "仅支持导入 .sql 备份文件"})
		return
	}

	maxUploadSize := getEnvInt64("BACKUP_UPLOAD_MAX_MB", 200) * 1024 * 1024
	if file.Size > maxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("备份文件过大，最大允许 %dMB", maxUploadSize/1024/1024)})
		return
	}

	tempFile, err := os.CreateTemp("", "erpwonchon-restore-*.sql")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建临时文件失败: " + err.Error()})
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	src, err := file.Open()
	if err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法打开上传的文件: " + err.Error()})
		return
	}
	defer src.Close()

	if _, err := io.Copy(tempFile, src); err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "文件保存失败: " + err.Error()})
		return
	}
	tempFile.Close()

	beforeCounts := collectRestoreCounts()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	args := mysqlClientArgs()
	cmd := exec.CommandContext(ctx, "mysql", args...)
	var stderr bytes.Buffer

	f, err := os.Open(tempPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法读取备份文件"})
		return
	}
	defer f.Close()
	cmd.Stdin = f
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "数据库恢复执行失败: " + stderr.String()})
		return
	}

	afterCounts := collectRestoreCounts()
	fmt.Printf("Backup restored: file=%s size=%d before=%v after=%v\n", file.Filename, file.Size, beforeCounts, afterCounts)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "数据库已成功从备份文件恢复",
		"data": gin.H{
			"file":   file.Filename,
			"size":   file.Size,
			"before": beforeCounts,
			"after":  afterCounts,
		},
	})
}

func mysqlClientArgs() []string {
	dbUser := getEnvString("DB_USER", "root")
	dbPass := os.Getenv("DB_PASS")
	dbHost := getEnvString("DB_HOST", "127.0.0.1")
	dbPort := getEnvString("DB_PORT", "3306")
	dbName := getEnvString("DB_NAME", "erp_db")

	args := []string{
		"--protocol=tcp",
		"-h", dbHost,
		"-P", dbPort,
		"-u", dbUser,
		"--default-character-set=utf8mb4",
		"--ssl=FALSE",
	}
	if dbPass != "" {
		args = append(args, "-p"+dbPass)
	}

	args = append(args, dbName)
	return args
}

func getEnvString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func collectRestoreCounts() gin.H {
	counts := gin.H{}
	for _, table := range []string{
		"accounts",
		"admin_users",
		"categories",
		"employees",
		"orders",
		"order_items",
		"products",
		"inventory_logs",
		"financial_logs",
		"ai_imports",
		"ai_provider_configs",
	} {
		var count int64
		if err := config.DB.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)).Scan(&count).Error; err != nil {
			counts[table] = "error: " + err.Error()
			continue
		}
		counts[table] = count
	}
	return counts
}
