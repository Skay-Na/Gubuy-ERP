package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
)

// ResetSystem 恢复出厂设置 - 清空所有业务数据
func ResetSystem(c *gin.Context) {
	// 这里可以加一个简单的校验，或者从 Header 传一个 RESET 标记
	var req struct {
		Confirm string `json:"confirm" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请正确输入确认码"})
		return
	}

	// 开启事务执行清空
	tx := config.DB.Begin()

	// 清空所有业务表 (按照依赖关系逆序删除或直接 TRUNCATE)
	// 注意：MySQL 中如果有外键约束，TRUNCATE 可能会失败，可以使用 DELETE
	tables := []interface{}{
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
func ExportBackup(c *gin.Context) {
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbHost := os.Getenv("DB_HOST")
	dbName := os.Getenv("DB_NAME")

	// 如果环境变量没设置，尝试使用默认值 (对应本地运行环境)
	if dbUser == "" {
		dbUser = "root"
	}
	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	if dbName == "" {
		dbName = "erp_db"
	}

	// 构造 mysqldump 命令
	// 注意：-p 和密码之间不能有空格
	args := []string{
		"-h", dbHost,
		"-u", dbUser,
		"--ssl=FALSE",
	}
	if dbPass != "" {
		args = append(args, "-p"+dbPass)
	}
	args = append(args, dbName)

	cmd := exec.Command("mysqldump", args...)

	// 设置响应头
	filename := fmt.Sprintf("gubuy_erp_backup_%s.sql", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Transfer-Encoding", "binary")

	// 将 mysqldump 的标准输出直接导向 Gin 的 Response Writer
	cmd.Stdout = c.Writer
	
	// 捕获错误输出并打印到日志 (可选)
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "备份启动失败: " + err.Error()})
		return
	}

	if err := cmd.Wait(); err != nil {
		// 这里如果已经开始写入 Body，JSON 响应可能无法正常显示，但在开发调试阶段很有用
		fmt.Printf("Mysqldump failed: %v\n", err)
		return
	}
}

// ImportBackup 导入数据库备份 (覆盖式)
func ImportBackup(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未接收到上传的文件"})
		return
	}

	// 将上传的文件保存到临时路径
	tempPath := "./temp_restore.sql"
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "文件保存失败: " + err.Error()})
		return
	}
	defer os.Remove(tempPath) // 函数结束时删除临时文件

	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbHost := os.Getenv("DB_HOST")
	dbName := os.Getenv("DB_NAME")

	if dbUser == "" {
		dbUser = "root"
	}
	if dbHost == "" {
		dbHost = "127.0.0.1"
	}
	if dbName == "" {
		dbName = "erp_db"
	}

	// 构造 mysql 导入命令
	args := []string{
		"-h", dbHost,
		"-u", dbUser,
		"--ssl=FALSE",
	}
	if dbPass != "" {
		args = append(args, "-p"+dbPass)
	}
	args = append(args, dbName)

	cmd := exec.Command("mysql", args...)

	// 打开临时文件并作为标准输入传给 mysql 命令
	f, err := os.Open(tempPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "无法读取备份文件"})
		return
	}
	defer f.Close()
	cmd.Stdin = f
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "数据库恢复执行失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "数据库已成功从备份文件恢复"})
}
