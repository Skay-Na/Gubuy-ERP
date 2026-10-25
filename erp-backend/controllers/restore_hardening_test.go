package controllers

// restore 接口加固专项回归（对应 commit 0f033ed）：
// 1. 不带 admin_password / 密码错误必须拒绝且数据库零变动；
// 2. 恢复前自动快照失败必须中止恢复（可回退性保证）；
// 3. 恢复成功后 operation_histories 必须留痕（文件/快照/前后行数）；
// 4. 响应携带恢复前后行数对照与快照路径。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const qaRestoreAdminPassword = "Qa-Restore-2026"

// ensureRestoreAdmin 重建一个密码已知的管理员账户。
// verifyRestoreAdminPassword 取第一条 admin 记录校验，测试库中先清空再插入，
// 与 ai_agent_cli_test 的管理员治理方式保持一致。
func ensureRestoreAdmin(t *testing.T) {
	t.Helper()
	if err := config.DB.Where("1 = 1").Delete(&models.AdminUser{}).Error; err != nil {
		t.Fatalf("清理管理员账户失败: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(qaRestoreAdminPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	admin := models.AdminUser{Username: "qa_restore_admin", Password: string(hash)}
	if err := config.DB.Create(&admin).Error; err != nil {
		t.Fatalf("创建测试管理员失败: %v", err)
	}
}

// buildRestoreMultipart 构造 restore 请求的 multipart 表单。
// adminPassword 为空时省略该字段（模拟未携带密码的客户端）。
func buildRestoreMultipart(t *testing.T, filename, content, adminPassword string) (*bytes.Buffer, string) {
	t.Helper()
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	file, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造上传文件失败: %v", err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatalf("写入上传内容失败: %v", err)
	}
	if adminPassword != "" {
		if err := form.WriteField("admin_password", adminPassword); err != nil {
			t.Fatalf("写入 admin_password 失败: %v", err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("关闭表单失败: %v", err)
	}
	return &upload, form.FormDataContentType()
}

// callRestoreImport 直接以测试上下文调用 ImportBackup。
func callRestoreImport(upload *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/api/system/restore", upload)
	ctx.Request.Header.Set("Content-Type", contentType)
	ImportBackup(ctx)
	return w
}

func restoreHistoryCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := config.DB.Model(&models.OperationHistory{}).
		Where("path = ? AND title = ?", "/api/system/restore", "恢复数据库备份").
		Count(&n).Error; err != nil {
		t.Fatalf("统计恢复操作历史失败: %v", err)
	}
	return n
}

// restoreMarkerProduct 放一行业务数据作为"数据库未被破坏"的标记。
func restoreMarkerProduct(t *testing.T, name string) {
	t.Helper()
	ensureProduct(t, name, 100.00, 60, 10)
}

func assertRestoreMarkerAlive(t *testing.T, name string) {
	t.Helper()
	var count int64
	if err := config.DB.Model(&models.Product{}).Where("name = ?", name).Count(&count).Error; err != nil {
		t.Fatalf("查询标记商品失败: %v", err)
	}
	if count == 0 {
		t.Fatal("标记商品丢失：恢复操作破坏了当前数据库数据")
	}
}

// TestRestoreWithoutAdminPasswordRejected 不带 admin_password 的请求必须被拒绝，
// 数据库零变动、不写操作历史、不留快照。
// 注：当前实现对"缺失密码"返回 400（参数缺失），对"密码错误"返回 401；
// 若验收口径要求缺失也返回 401，需由工程师调整后同步本用例。
func TestRestoreWithoutAdminPasswordRejected(t *testing.T) {
	setupAudit(t)
	ensureRestoreAdmin(t)
	restoreMarkerProduct(t, "恢复加固标记商品-缺失密码")
	historyBefore := restoreHistoryCount(t)
	backupDir := t.TempDir()
	t.Setenv("BACKUP_DIR", backupDir)

	upload, contentType := buildRestoreMultipart(t, "qa-no-password.sql", "-- no-op sql", "")
	w := callRestoreImport(upload, contentType)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺失 admin_password 应被拒绝(400/401)，实际 %d %s", w.Code, w.Body.String())
	}
	assertRestoreMarkerAlive(t, "恢复加固标记商品-缺失密码")
	if got := restoreHistoryCount(t); got != historyBefore {
		t.Fatalf("被拒绝的恢复不应写操作历史：before=%d after=%d", historyBefore, got)
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 0 {
		t.Fatalf("被拒绝的恢复不应生成快照，目录内发现 %d 个文件", len(entries))
	}
}

// TestRestoreWithWrongAdminPasswordRejected 密码错误必须返回 401，
// 数据库零变动、不写操作历史、不留快照。
func TestRestoreWithWrongAdminPasswordRejected(t *testing.T) {
	setupAudit(t)
	ensureRestoreAdmin(t)
	restoreMarkerProduct(t, "恢复加固标记商品-错误密码")
	historyBefore := restoreHistoryCount(t)
	backupDir := t.TempDir()
	t.Setenv("BACKUP_DIR", backupDir)

	upload, contentType := buildRestoreMultipart(t, "qa-wrong-password.sql", "-- no-op sql", "definitely-wrong-password")
	w := callRestoreImport(upload, contentType)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("管理员密码错误应返回 401，实际 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "管理员密码错误") {
		t.Fatalf("响应应说明密码错误，实际: %s", w.Body.String())
	}
	assertRestoreMarkerAlive(t, "恢复加固标记商品-错误密码")
	if got := restoreHistoryCount(t); got != historyBefore {
		t.Fatalf("被拒绝的恢复不应写操作历史：before=%d after=%d", historyBefore, got)
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 0 {
		t.Fatalf("被拒绝的恢复不应生成快照，目录内发现 %d 个文件", len(entries))
	}
}

// TestRestoreAbortedWhenSnapshotFails 快照目录不可写（BACKUP_DIR 指向
// 已存在文件的子路径）时，恢复必须中止：返回 500、数据零变动、不写历史。
func TestRestoreAbortedWhenSnapshotFails(t *testing.T) {
	setupAudit(t)
	ensureRestoreAdmin(t)
	restoreMarkerProduct(t, "恢复加固标记商品-快照失败")
	historyBefore := restoreHistoryCount(t)

	// 构造 MkdirAll 必然失败的 BACKUP_DIR：父路径是一个普通文件
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("创建占位文件失败: %v", err)
	}
	t.Setenv("BACKUP_DIR", filepath.Join(blocker, "sub"))

	// 内容为无害 SQL：若中止逻辑失效，也不会破坏测试库
	upload, contentType := buildRestoreMultipart(t, "qa-snapshot-fail.sql", "SELECT 1;", qaRestoreAdminPassword)
	w := callRestoreImport(upload, contentType)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("快照失败应中止恢复并返回 500，实际 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "恢复已中止") {
		t.Fatalf("响应应说明恢复已中止，实际: %s", w.Body.String())
	}
	assertRestoreMarkerAlive(t, "恢复加固标记商品-快照失败")
	if got := restoreHistoryCount(t); got != historyBefore {
		t.Fatalf("中止的恢复不应写操作历史：before=%d after=%d", historyBefore, got)
	}
}

// TestRestoreSuccessRecordsOperationHistory 成功恢复的完整链路：
// 密码校验 → 自动快照落盘 → 恢复到隔离库 → operation_histories 留痕
// → 响应携带 before/after 行数对照与快照路径。
// 无可用数据库客户端（本地 PATH 或 docker 容器）时跳过。
func TestRestoreSuccessRecordsOperationHistory(t *testing.T) {
	if os.Getenv("ERP_AUDIT_DB") != "1" || os.Getenv("DB_NAME") != "erp_audit_test" {
		t.Fatal("restore 集成用例仅允许在隔离的 erp_audit_test 库执行")
	}
	if _, err := findMySQLCommand("mariadb-dump", "mysqldump"); err != nil {
		t.Skip(err.Error())
	}
	restoreClient, err := findMySQLCommand("mariadb", "mysql")
	if err != nil {
		t.Skip(err.Error())
	}

	setupAudit(t)
	ensureRestoreAdmin(t)
	const markerName = "恢复加固标记商品-成功链路"
	restoreMarkerProduct(t, markerName)
	historyBefore := restoreHistoryCount(t)

	// 导出当前测试库作为待恢复的备份文件
	exportRecorder := httptest.NewRecorder()
	exportContext, _ := gin.CreateTestContext(exportRecorder)
	exportContext.Request = httptest.NewRequest("GET", "/api/system/backup", nil)
	ExportBackup(exportContext)
	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("导出备份失败: %d %s", exportRecorder.Code, exportRecorder.Body.String())
	}
	dump := append([]byte(nil), exportRecorder.Body.Bytes()...)

	// 建隔离目标库，恢复动作只写入隔离库，不污染测试库
	targetDatabase := fmt.Sprintf("erp_restore_qa_test_%d", time.Now().UnixNano())
	adminArgs := mysqlClientArgs(restoreClient)
	adminArgs = append(adminArgs[:len(adminArgs)-1], "--execute", "CREATE DATABASE `"+targetDatabase+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if output, err := exec.Command(restoreClient.execPath, restoreClient.fullArgs(adminArgs)...).CombinedOutput(); err != nil {
		t.Fatalf("创建隔离恢复库失败: %v (%s)", err, output)
	}
	t.Cleanup(func() {
		args := mysqlClientArgs(restoreClient)
		args = append(args[:len(args)-1], "--execute", "DROP DATABASE IF EXISTS `"+targetDatabase+"`")
		_, _ = exec.Command(restoreClient.execPath, restoreClient.fullArgs(args)...).CombinedOutput()
	})

	backupDir := t.TempDir()
	t.Setenv("BACKUP_DIR", backupDir)
	t.Setenv("DB_NAME", targetDatabase) // mysqlClientArgs 在调用时读取，恢复只写隔离库

	upload, contentType := buildRestoreMultipart(t, "qa-restore.sql", string(dump), qaRestoreAdminPassword)
	w := callRestoreImport(upload, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("携带正确密码的恢复应成功，实际 %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			File     string `json:"file"`
			Snapshot string `json:"snapshot"`
			Before   gin.H  `json:"before"`
			After    gin.H  `json:"after"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Snapshot == "" {
		t.Fatalf("响应应携带快照路径: %s", w.Body.String())
	}
	// 快照文件必须真实落盘在 BACKUP_DIR
	if _, err := os.Stat(resp.Data.Snapshot); err != nil {
		t.Fatalf("快照文件未落盘(%s): %v", resp.Data.Snapshot, err)
	}
	if !strings.HasPrefix(filepath.Clean(resp.Data.Snapshot), filepath.Clean(backupDir)) {
		t.Fatalf("快照应写入 BACKUP_DIR(%s)，实际 %s", backupDir, resp.Data.Snapshot)
	}
	if len(resp.Data.Before) == 0 || len(resp.Data.After) == 0 {
		t.Fatalf("响应应携带恢复前后行数对照: %s", w.Body.String())
	}

	// operation_histories 必须新增一条恢复记录（写入 config.DB 连接的测试库）
	if got := restoreHistoryCount(t); got != historyBefore+1 {
		t.Fatalf("成功恢复应写一条操作历史：before=%d after=%d", historyBefore, got)
	}
	var entry models.OperationHistory
	if err := config.DB.Where("path = ? AND title = ?", "/api/system/restore", "恢复数据库备份").
		Order("id DESC").First(&entry).Error; err != nil {
		t.Fatalf("读取恢复操作历史失败: %v", err)
	}
	// Changes 为 JSON 摘要（Windows 路径含转义反斜杠，须解析后比对字段）
	var changes struct {
		File     string `json:"file"`
		Snapshot string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(entry.Changes), &changes); err != nil {
		t.Fatalf("操作历史 Changes 应为合法 JSON: %v（实际: %s）", err, entry.Changes)
	}
	if changes.File != "qa-restore.sql" {
		t.Fatalf("操作历史应记录恢复文件名 qa-restore.sql，实际: %s", changes.File)
	}
	if changes.Snapshot != resp.Data.Snapshot {
		t.Fatalf("操作历史应记录快照路径 %s，实际: %s", resp.Data.Snapshot, changes.Snapshot)
	}

	// 隔离库应真的恢复了数据：标记商品存在
	query := fmt.Sprintf("SELECT COUNT(*) FROM products WHERE name = '%s'", markerName)
	args := mysqlClientArgs(restoreClient)
	args = append(args[:len(args)-1], "--batch", "--skip-column-names", "--execute", query, targetDatabase)
	output, err := exec.Command(restoreClient.execPath, restoreClient.fullArgs(args)...).Output()
	if err != nil {
		t.Fatalf("查询隔离库失败: %v", err)
	}
	if strings.TrimSpace(string(output)) == "0" {
		t.Fatal("隔离库中未找到标记商品，恢复未真正生效")
	}
}
