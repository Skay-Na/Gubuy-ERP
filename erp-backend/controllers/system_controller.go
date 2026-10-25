package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
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
		&models.EmployeeOffboardingAlert{},
		&models.EmployeeHandoverTask{},
		&models.EmployeeOffboarding{},
		&models.AIProviderConfig{},
		&models.AIImport{},
		&models.AIAgentAttachment{},
		&models.AIAgentMessage{},
		&models.AIAgentAction{},
		&models.AIAgentSession{},
		&models.FinancialLog{},
		&models.InventoryLog{},
		&models.TransferRecord{},
		&models.StocktakeRecord{},
		&models.InboundRecord{},
		&models.Expense{},
		&models.SubsidyReceipt{},
		&models.SubsidySettlement{},
		&models.OrderItem{},
		&models.Order{},
		&models.Product{},
		&models.Category{},
		&models.MonthlyAttendance{},
		&models.DailyAttendance{},
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

	dumpCommand, err := findMySQLCommand("mariadb-dump", "mysqldump")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "数据库备份执行失败: " + err.Error()})
		return
	}

	tempFile, err := os.CreateTemp("", "erpwonchon-backup-*.sql")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建备份临时文件失败: " + err.Error()})
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	args := []string{"--single-transaction", "--quick", "--hex-blob", "--no-tablespaces"}
	args = dumpCommand.fullArgs(mysqlClientArgs(dumpCommand, args...))
	cmd := exec.CommandContext(ctx, dumpCommand.execPath, args...)
	var stderr bytes.Buffer
	cmd.Stdout = tempFile
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		tempFile.Close()
		c.JSON(
			http.StatusInternalServerError,
			gin.H{"code": 500, "msg": "数据库备份执行失败: " + mysqlCommandError(stderr.String(), err)},
		)
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
// @Description 导入数据备份（需管理员密码二次确认；恢复前自动落盘全库快照）
// @Tags System
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "SQL 备份文件"
// @Param admin_password formData string true "管理员密码（二次确认）"
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

	// 二次确认：必须携带管理员密码并通过校验，才允许执行覆盖式恢复。
	adminPassword := c.PostForm("admin_password")
	if adminPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请输入管理员密码以确认恢复操作"})
		return
	}
	if !verifyRestoreAdminPassword(adminPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "管理员密码错误，已取消恢复操作"})
		return
	}

	maxUploadSize := getEnvInt64("BACKUP_UPLOAD_MAX_MB", 200) * 1024 * 1024
	if file.Size > maxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("备份文件过大，最大允许 %dMB", maxUploadSize/1024/1024)})
		return
	}

	restoreCommand, err := findMySQLCommand("mariadb", "mysql")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "数据库恢复执行失败: " + err.Error()})
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
	if err := tempFile.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "备份文件保存失败: " + err.Error()})
		return
	}

	beforeCounts := collectRestoreCounts()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	// 恢复前强制自动快照：复用备份导出机制落盘一份全库 SQL 到服务器备份目录，
	// 失败则中止恢复，保证误恢复时仍可回退。
	snapshotPath, err := createPreRestoreSnapshot(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "恢复已中止：" + err.Error()})
		return
	}

	args := restoreCommand.fullArgs(mysqlClientArgs(restoreCommand))
	cmd := exec.CommandContext(ctx, restoreCommand.execPath, args...)
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
		c.JSON(
			http.StatusInternalServerError,
			gin.H{"code": 500, "msg": "数据库恢复执行失败: " + mysqlCommandError(stderr.String(), err)},
		)
		return
	}

	afterCounts := collectRestoreCounts()
	fmt.Printf("Backup restored: file=%s size=%d snapshot=%s before=%v after=%v\n",
		file.Filename, file.Size, snapshotPath, beforeCounts, afterCounts)

	// 恢复动作写入操作历史（该路由未纳入 OperationHistoryMiddleware 跟踪表，手工补记）。
	recordRestoreOperationHistory(c, file.Filename, file.Size, snapshotPath, beforeCounts, afterCounts)

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "数据库已成功从备份文件恢复",
		"data": gin.H{
			"file":     file.Filename,
			"size":     file.Size,
			"snapshot": snapshotPath,
			"before":   beforeCounts,
			"after":    afterCounts,
		},
	})
}

// createPreRestoreSnapshot 在恢复前对当前库做一次全量 mysqldump 快照，
// 落盘到 BACKUP_DIR（默认 ./backups）目录，返回快照文件路径。
// 机制与 ExportBackup 完全一致（同一套 mysqlClientCommand 查找/包装逻辑）。
func createPreRestoreSnapshot(ctx context.Context) (string, error) {
	dumpCommand, err := findMySQLCommand("mariadb-dump", "mysqldump")
	if err != nil {
		return "", fmt.Errorf("数据库备份执行失败: %w", err)
	}

	backupDir := getEnvString("BACKUP_DIR", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}
	snapshotPath := filepath.Join(backupDir, fmt.Sprintf("pre_restore_snapshot_%s.sql", time.Now().Format("20060102_150405")))

	snapshotFile, err := os.Create(snapshotPath)
	if err != nil {
		return "", fmt.Errorf("创建快照文件失败: %w", err)
	}

	args := []string{"--single-transaction", "--quick", "--hex-blob", "--no-tablespaces"}
	args = dumpCommand.fullArgs(mysqlClientArgs(dumpCommand, args...))
	cmd := exec.CommandContext(ctx, dumpCommand.execPath, args...)
	var stderr bytes.Buffer
	cmd.Stdout = snapshotFile
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		snapshotFile.Close()
		os.Remove(snapshotPath)
		return "", fmt.Errorf("恢复前自动备份失败: %s", mysqlCommandError(stderr.String(), err))
	}
	if err := snapshotFile.Close(); err != nil {
		os.Remove(snapshotPath)
		return "", fmt.Errorf("快照文件写入失败: %w", err)
	}
	return snapshotPath, nil
}

// verifyRestoreAdminPassword 校验恢复操作的管理员密码，
// 与 VerifyAdminPassword 登录验证保持一致的 bcrypt/明文兼容逻辑。
func verifyRestoreAdminPassword(password string) bool {
	var admin models.AdminUser
	if err := config.DB.First(&admin).Error; err != nil {
		return false
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err == nil {
		return true
	}
	// 兼容历史明文密码（登录逻辑同样保留该兼容分支）
	return admin.Password == password
}

// recordRestoreOperationHistory 将一次恢复操作写入 operation_histories，
// Changes 存放恢复摘要（文件、快照路径、前后行数对照），便于事后审计。
func recordRestoreOperationHistory(c *gin.Context, filename string, size int64, snapshotPath string, before, after gin.H) {
	summary, err := json.Marshal(gin.H{
		"file":     filename,
		"size":     size,
		"snapshot": snapshotPath,
		"before":   before,
		"after":    after,
	})
	if err != nil {
		summary = []byte(fmt.Sprintf(`{"file":%q,"snapshot":%q}`, filename, snapshotPath))
	}
	entry := models.OperationHistory{
		Username: c.GetString("username"),
		Title:    "恢复数据库备份",
		Method:   c.Request.Method,
		Path:     "/api/system/restore",
		Changes:  string(summary),
	}
	if err := config.DB.Create(&entry).Error; err != nil {
		fmt.Printf("restore operation history save failed: %v\n", err)
	}
}

// mysqlClientCommand 描述一次数据库客户端调用：
// - 本地模式：直接执行宿主机上的客户端（execPath 即客户端路径，prefix 为空）
// - Docker 模式：通过 docker exec 进入数据库容器执行（execPath 为 docker，prefix 为 exec 包装参数）
type mysqlClientCommand struct {
	execPath string   // 宿主机可执行文件（本地客户端或 docker 本身）
	prefix   []string // 附加在客户端参数之前的包装参数（如 docker exec gubuy-erp-db）
	binary   string   // 客户端二进制名，用于区分 mariadb/mysql 的 SSL 参数
	host     string   // 客户端连接时使用的主机
	port     string   // 客户端连接时使用的端口
}

// fullArgs 组装完整命令参数：包装参数 + [客户端名] + 客户端参数。
// 本地模式 prefix 为空，execPath 本身就是客户端；
// Docker 模式需要在 docker exec <容器> 之后补上容器内的客户端命令名。
func (m mysqlClientCommand) fullArgs(clientArgs []string) []string {
	args := make([]string, 0, len(m.prefix)+1+len(clientArgs))
	args = append(args, m.prefix...)
	if len(m.prefix) > 0 {
		args = append(args, m.binary)
	}
	return append(args, clientArgs...)
}

func mysqlClientArgs(client mysqlClientCommand, extraOptions ...string) []string {
	dbUser := getEnvString("DB_USER", "root")
	dbPass := os.Getenv("DB_PASS")
	dbName := getEnvString("DB_NAME", "erp_db")

	sslOption := "--ssl-mode=DISABLED"
	if strings.Contains(client.binary, "mariadb") {
		sslOption = "--skip-ssl"
	}

	args := []string{
		"--protocol=tcp",
		"-h", client.host,
		"-P", client.port,
		"-u", dbUser,
		"--default-character-set=utf8mb4",
		sslOption,
	}
	args = append(args, extraOptions...)
	if dbPass != "" {
		args = append(args, "-p"+dbPass)
	}

	args = append(args, dbName)
	return args
}

func mysqlCommandError(stderr string, err error) string {
	detail := strings.TrimSpace(stderr)
	if detail != "" {
		return detail
	}

	return err.Error()
}

// findMySQLCommand 查找可用的数据库客户端：
// 1. 优先在宿主机 PATH 中查找本地客户端（mariadb/mariadb-dump 或 mysql/mysqldump）
// 2. 找不到时降级为 docker exec 进入数据库容器执行（DB_CLIENT_MODE=local 可强制关闭）
func findMySQLCommand(names ...string) (mysqlClientCommand, error) {
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return mysqlClientCommand{
				execPath: path,
				binary:   name,
				host:     getEnvString("DB_HOST", "127.0.0.1"),
				port:     getEnvString("DB_PORT", "3306"),
			}, nil
		}
	}

	if getEnvString("DB_CLIENT_MODE", "auto") != "local" {
		if client, dockerErr := findDockerMySQLClient(names...); dockerErr == nil {
			return client, nil
		}
	}

	return mysqlClientCommand{}, fmt.Errorf(
		"未找到数据库客户端命令（尝试了 %s），请在后端运行环境安装 MariaDB 或 MySQL 客户端，或确保数据库容器（Docker）正在运行",
		strings.Join(names, "、"),
	)
}

// findDockerMySQLClient 在数据库容器内查找客户端命令。
// 容器名默认 gubuy-erp-db，可用 DB_DOCKER_CONTAINER 覆盖；
// 容器内连接地址默认 127.0.0.1:3306，可用 DB_DOCKER_HOST / DB_DOCKER_PORT 覆盖。
func findDockerMySQLClient(names ...string) (mysqlClientCommand, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return mysqlClientCommand{}, fmt.Errorf("未找到 docker 命令，无法进入数据库容器执行")
	}

	container := getEnvString("DB_DOCKER_CONTAINER", "gubuy-erp-db")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 先确认容器在运行，给出比 "exec: xxx" 更明确的错误
	inspect := exec.CommandContext(ctx, dockerPath, "inspect", "-f", "{{.State.Running}}", container)
	if output, inspectErr := inspect.Output(); inspectErr != nil || strings.TrimSpace(string(output)) != "true" {
		return mysqlClientCommand{}, fmt.Errorf("数据库容器 %s 未在运行，无法通过 Docker 执行数据库客户端", container)
	}

	var lastErr error
	for _, name := range names {
		probe := exec.CommandContext(ctx, dockerPath, "exec", container, name, "--version")
		if err := probe.Run(); err == nil {
			return mysqlClientCommand{
				execPath: dockerPath,
				// -i 保持 stdin 打开：恢复导入需要从宿主机管道读入 SQL 流，
				// 备份导出时也无副作用
				prefix:   []string{"exec", "-i", container},
				binary:   name,
				host:     getEnvString("DB_DOCKER_HOST", "127.0.0.1"),
				port:     getEnvString("DB_DOCKER_PORT", "3306"),
			}, nil
		} else {
			lastErr = err
		}
	}

	if lastErr != nil {
		return mysqlClientCommand{}, fmt.Errorf("数据库容器 %s 内未找到数据库客户端命令: %w", container, lastErr)
	}
	return mysqlClientCommand{}, fmt.Errorf("数据库容器 %s 内未找到数据库客户端命令", container)
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
		"employee_offboardings",
		"employee_handover_tasks",
		"employee_offboarding_alerts",
		"daily_attendances",
		"orders",
		"order_items",
		"products",
		"inventory_logs",
		"financial_logs",
		"ai_imports",
		"ai_provider_configs",
		"ai_agent_attachments",
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
