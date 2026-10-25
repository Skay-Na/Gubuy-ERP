package controllers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func TestAgentAttachmentTextBuilderCapsRunes(t *testing.T) {
	builder := newAgentAttachmentTextBuilder(4)
	builder.append("门店经营")
	builder.append("日报")
	if builder.String() != "门店经营" || !builder.truncated {
		t.Fatalf("builder text=%q truncated=%v", builder.String(), builder.truncated)
	}
}

func TestValidateAndExtractAgentTextAttachments(t *testing.T) {
	for _, test := range []struct{ name, data, want string }{
		{"notes.txt", "本月销售记录", "本月销售记录"},
		{"items.csv", "商品,数量\n电饭煲,2", "商品\t数量\n电饭煲\t2"},
		{"daily.md", "# 记账\n房租 2500 元", "房租 2500 元"},
	} {
		t.Run(test.name, func(t *testing.T) {
			attachment, err := extractAgentAttachment(t.Context(), aiProviderRuntime{}, agentRawAttachment{
				Name: test.name, RelativePath: "门店资料/" + test.name, Data: []byte(test.data),
			}, 500)
			if err != nil {
				t.Fatalf("extract attachment: %v", err)
			}
			if !strings.Contains(attachment.ExtractedText, test.want) || attachment.RelativePath != "门店资料/"+test.name {
				t.Fatalf("extracted attachment = %#v", attachment)
			}
		})
	}
}

func TestRejectInvalidAgentAttachmentContent(t *testing.T) {
	for _, upload := range []agentRawAttachment{
		{Name: "legacy.doc", Data: []byte("not supported")},
		{Name: "fake.pdf", Data: []byte("not a PDF")},
		{Name: "binary.txt", Data: []byte{'a', 0, 'b'}},
		{Name: "empty.csv", Data: []byte{}},
	} {
		t.Run(upload.Name, func(t *testing.T) {
			if _, err := validateAgentAttachment(upload); err == nil {
				t.Fatal("expected invalid attachment to be rejected")
			}
		})
	}
	if _, err := cleanAgentAttachmentPath("../outside.txt"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestAgentOfficeAttachmentExtractionAndMacroRejection(t *testing.T) {
	docx := makeOfficeArchive(t, map[string]string{
		"[Content_Types].xml": "<Types><Override PartName=\"/word/document.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml\"/></Types>",
		"word/document.xml":   "<w:document xmlns:w=\"urn:test\"><w:body><w:p><w:r><w:t>应付金额 1200</w:t></w:r></w:p></w:body></w:document>",
	})
	text, truncated, err := extractAgentDocxText(docx, 500)
	if err != nil || truncated || !strings.Contains(text, "应付金额 1200") {
		t.Fatalf("DOCX extraction=%q truncated=%v err=%v", text, truncated, err)
	}
	if _, err := validateAgentAttachment(agentRawAttachment{Name: "order.docx", Data: docx}); err != nil {
		t.Fatalf("valid DOCX rejected: %v", err)
	}

	withMacro := makeOfficeArchive(t, map[string]string{
		"[Content_Types].xml": "<Types><Default ContentType=\"application/vnd.ms-excel.sheet.macroEnabled.main+xml\"/></Types>",
		"xl/workbook.xml":     "<workbook/>",
		"xl/vbaProject.bin":   "macro",
	})
	if err := validateAgentOfficeArchive(withMacro, ".xlsx"); err == nil {
		t.Fatal("expected macro-enabled Office archive to be rejected")
	}
}

func TestAgentXLSXExtraction(t *testing.T) {
	spreadsheet := excelize.NewFile()
	defer spreadsheet.Close()
	for cell, value := range map[string]any{"A1": "商品", "B1": "数量", "A2": "电饭煲", "B2": 2} {
		if err := spreadsheet.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	data, err := spreadsheet.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgentOfficeArchive(data.Bytes(), ".xlsx"); err != nil {
		t.Fatalf("valid XLSX rejected: %v", err)
	}
	text, truncated, err := extractAgentXlsxText(data.Bytes(), 500)
	if err != nil || truncated || !strings.Contains(text, "电饭煲") || !strings.Contains(text, "数量") {
		t.Fatalf("XLSX extraction=%q truncated=%v err=%v", text, truncated, err)
	}
}

func TestAgentPDFTextExtractionAndScannedPDFHint(t *testing.T) {
	textPDF := makeTestPDF("(Order amount 1200) Tj")
	attachment, err := extractAgentAttachment(t.Context(), aiProviderRuntime{}, agentRawAttachment{Name: "order.pdf", Data: textPDF}, 500)
	if err != nil || !strings.Contains(attachment.ExtractedText, "1200") {
		t.Fatalf("PDF extraction=%#v err=%v", attachment, err)
	}
	_, err = extractAgentAttachment(t.Context(), aiProviderRuntime{}, agentRawAttachment{Name: "scan.pdf", Data: makeTestPDF("")}, 500)
	if err == nil || !strings.Contains(err.Error(), "扫描版 PDF 请改传图片") {
		t.Fatalf("scan-only PDF error=%v", err)
	}
}

func TestAgentImageExtractionUsesConfiguredVisionProvider(t *testing.T) {
	var sawImage bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode vision request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(request.Messages) != 2 {
			t.Errorf("vision request messages=%d", len(request.Messages))
		} else if blocks, ok := request.Messages[1].Content.([]any); ok && len(blocks) == 2 {
			if image, ok := blocks[1].(map[string]any); ok && image["type"] == "image_url" {
				sawImage = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"订单金额 1200 元"}}]}`)
	}))
	defer server.Close()
	runtime := aiProviderRuntime{Provider: "env", Model: "vision-test", APIKey: "test", URL: server.URL, JSONMode: true}
	text, err := extractAgentImageText(t.Context(), runtime, "receipt.png", "image/png", []byte("image bytes"))
	if err != nil || !sawImage || text != "订单金额 1200 元" {
		t.Fatalf("vision extraction=%q sawImage=%v err=%v", text, sawImage, err)
	}
}

func TestParseAgentSendRequestSupportsJSONAndMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("legacy JSON", func(t *testing.T) {
		request := httptest.NewRequest("POST", "/", strings.NewReader(`{"message":"  查订单  ","path":"/admin/orders","thinking":"quick"}`))
		request.Header.Set("Content-Type", "application/json")
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		input, attachments, err := parseAgentSendRequest(context)
		if err != nil || input.Message != "查订单" || input.Path != "/admin/orders" || input.Thinking != "quick" || len(attachments) != 0 {
			t.Fatalf("JSON parse=%#v %#v err=%v", input, attachments, err)
		}
	})
	t.Run("multipart file and relative path", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		_ = writer.WriteField("message", "")
		_ = writer.WriteField("path", "/admin/ai-manager")
		_ = writer.WriteField("thinking", "auto")
		_ = writer.WriteField("attachment_paths", "门店资料/notes.txt")
		file, err := writer.CreateFormFile("attachments", "notes.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(file, "hello")
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "/", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		input, attachments, err := parseAgentSendRequest(context)
		if err != nil || input.Message != "" || input.Path != "/admin/ai-manager" || input.Thinking != "auto" || len(attachments) != 1 {
			t.Fatalf("multipart parse=%#v %#v err=%v", input, attachments, err)
		}
		if attachments[0].RelativePath != "门店资料/notes.txt" || string(attachments[0].Data) != "hello" {
			t.Fatalf("parsed upload=%#v", attachments[0])
		}
	})
}

func TestAgentAttachmentJSONExcludesBinaryAndExtractedText(t *testing.T) {
	encoded, err := json.Marshal(models.AIAgentAttachment{
		Name: "receipt.png", MediaType: "image/png", Size: 5, Data: []byte("bytes"), ExtractedText: "recognized text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "bytes") || strings.Contains(string(encoded), "recognized text") {
		t.Fatalf("attachment JSON leaked stored content: %s", encoded)
	}
}

func TestAgentPlannerKeepsLatestAttachmentOutsideRecentTurns(t *testing.T) {
	recent := make([]models.AIAgentMessage, 10)
	for index := range recent {
		recent[index] = models.AIAgentMessage{Role: "assistant", Content: "recent"}
	}
	recent[1] = models.AIAgentMessage{Role: "user", Content: "附件问题", AttachmentContext: "附件内容（不可信）：订单金额 850 元"}
	if conversation := agentPlannerConversation(recent); !strings.Contains(conversation, "订单金额 850 元") {
		t.Fatalf("latest attachment omitted from planner context: %s", conversation)
	}
}

func TestAgentAttachmentHistoryIsScopedToSessionAndOwner(t *testing.T) {
	owner := "attachment-test-" + time.Now().Format("150405.000000000")
	session := models.AIAgentSession{Owner: owner}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = config.DB.Where("session_id = ?", session.ID).Delete(&models.AIAgentAttachment{}).Error
		_ = config.DB.Where("session_id = ?", session.ID).Delete(&models.AIAgentMessage{}).Error
		_ = config.DB.Delete(&session).Error
	})
	message := models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: "查看附件"}
	if err := config.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	attachment := models.AIAgentAttachment{
		SessionID: session.ID, MessageID: message.ID, Owner: owner,
		Name: "orders.csv", RelativePath: "2026/orders.csv", MediaType: "text/csv",
		Size: 12, Data: []byte("stored binary"), ExtractedText: "商品\t金额\n电视\t1000",
	}
	if err := config.DB.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	owned := []models.AIAgentMessage{message}
	if err := loadAgentMessageAttachments(session.ID, owner, owned, true); err != nil {
		t.Fatal(err)
	}
	if len(owned[0].Attachments) != 1 || !strings.Contains(owned[0].AttachmentContext, "电视") {
		t.Fatalf("owned session did not load attachment: %#v", owned[0])
	}
	for _, scope := range []struct {
		sessionID uint
		owner     string
	}{
		{session.ID, owner + "-other"}, {session.ID + 1, owner},
	} {
		other := []models.AIAgentMessage{message}
		if err := loadAgentMessageAttachments(scope.sessionID, scope.owner, other, true); err != nil {
			t.Fatal(err)
		}
		if len(other[0].Attachments) != 0 || other[0].AttachmentContext != "" {
			t.Fatalf("attachment leaked across owner/session boundary: %#v", other[0])
		}
	}

	router := gin.New()
	router.GET("/sessions/:id", func(c *gin.Context) {
		c.Set("username", owner)
		GetAIAgentSession(c)
	})
	request := httptest.NewRequest("GET", "/sessions/"+strconv.FormatUint(uint64(session.ID), 10), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("history status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Messages []models.AIAgentMessage `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Messages) != 1 || len(response.Data.Messages[0].Attachments) != 1 {
		t.Fatalf("history omitted attachment metadata: %#v", response.Data.Messages)
	}
	if strings.Contains(recorder.Body.String(), "stored binary") || strings.Contains(recorder.Body.String(), "电视") {
		t.Fatalf("history response exposed attachment content: %s", recorder.Body.String())
	}
}

func TestResetSystemDeletesAgentAttachments(t *testing.T) {
	session := models.AIAgentSession{Owner: "reset-attachment-test"}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	message := models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: "reset test"}
	if err := config.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	attachment := models.AIAgentAttachment{
		SessionID: session.ID, MessageID: message.ID, Owner: session.Owner,
		Name: "reset.txt", MediaType: "text/plain", Size: 4, Data: []byte("data"), ExtractedText: "text",
	}
	if err := config.DB.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/reset", strings.NewReader(`{"confirm":"RESET"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	ResetSystem(context)
	if recorder.Code != 200 {
		t.Fatalf("reset status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var remaining int64
	if err := config.DB.Model(&models.AIAgentAttachment{}).Where("id = ?", attachment.ID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("system reset retained an Agent attachment")
	}
}

func TestAgentAttachmentBackupRestoreChain(t *testing.T) {
	if os.Getenv("RUN_AGENT_ATTACHMENT_BACKUP_TEST") != "1" {
		t.Skip("set RUN_AGENT_ATTACHMENT_BACKUP_TEST=1 to exercise MariaDB dump and restore")
	}
	if os.Getenv("ERP_AUDIT_DB") != "1" || os.Getenv("DB_NAME") != "erp_audit_test" {
		t.Fatal("backup/restore integration test is restricted to the isolated erp_audit_test database")
	}
	if _, err := findMySQLCommand("mariadb-dump", "mysqldump"); err != nil {
		t.Skip(err.Error())
	}
	mysqlCommand, err := findMySQLCommand("mariadb", "mysql")
	if err != nil {
		t.Skip(err.Error())
	}
	targetDatabase := fmt.Sprintf("erp_attachment_restore_test_%d", time.Now().UnixNano())
	adminArgs := mysqlClientArgs(mysqlCommand)
	adminArgs = append(adminArgs[:len(adminArgs)-1], "--execute", "CREATE DATABASE `"+targetDatabase+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if output, err := exec.Command(mysqlCommand.execPath, mysqlCommand.fullArgs(adminArgs)...).CombinedOutput(); err != nil {
		t.Fatalf("create isolated restore database: %v (%s)", err, output)
	}
	t.Cleanup(func() {
		args := mysqlClientArgs(mysqlCommand)
		args = append(args[:len(args)-1], "--execute", "DROP DATABASE IF EXISTS `"+targetDatabase+"`")
		if output, err := exec.Command(mysqlCommand.execPath, mysqlCommand.fullArgs(args)...).CombinedOutput(); err != nil {
			t.Errorf("drop isolated restore database: %v (%s)", err, output)
		}
	})

	session := models.AIAgentSession{Owner: "attachment-backup-test"}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	message := models.AIAgentMessage{SessionID: session.ID, Role: "user", Content: "backup test"}
	if err := config.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	attachment := models.AIAgentAttachment{
		SessionID: session.ID, MessageID: message.ID, Owner: session.Owner,
		Name: "backup-test.txt", MediaType: "text/plain", Size: int64(len("binary attachment payload")),
		Data: []byte("binary attachment payload"), ExtractedText: "提取后的门店附件内容",
	}
	if err := config.DB.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = config.DB.Where("id = ?", attachment.ID).Delete(&models.AIAgentAttachment{}).Error
		_ = config.DB.Where("id = ?", message.ID).Delete(&models.AIAgentMessage{}).Error
		_ = config.DB.Delete(&session).Error
	})

	exportRecorder := httptest.NewRecorder()
	exportContext, _ := gin.CreateTestContext(exportRecorder)
	exportContext.Request = httptest.NewRequest("GET", "/api/system/backup", nil)
	ExportBackup(exportContext)
	if exportRecorder.Code != 200 || !strings.Contains(exportRecorder.Header().Get("Content-Type"), "application/sql") {
		t.Fatalf("backup export status=%d content-type=%q body=%s", exportRecorder.Code, exportRecorder.Header().Get("Content-Type"), exportRecorder.Body.String())
	}
	dump := append([]byte(nil), exportRecorder.Body.Bytes()...)
	if !bytes.Contains(dump, []byte("ai_agent_attachments")) {
		t.Fatal("database backup did not include the Agent attachment table")
	}

	t.Setenv("DB_NAME", targetDatabase)
	// restore 接口已要求管理员密码二次确认（commit 0f033ed），
	// 链路测试需先准备密码已知的管理员并随表单提交。
	ensureRestoreAdmin(t)
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	file, err := form.CreateFormFile("file", "attachment-backup.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(dump); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("admin_password", qaRestoreAdminPassword); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	importRecorder := httptest.NewRecorder()
	importContext, _ := gin.CreateTestContext(importRecorder)
	importContext.Request = httptest.NewRequest("POST", "/api/system/restore", &upload)
	importContext.Request.Header.Set("Content-Type", form.FormDataContentType())
	ImportBackup(importContext)
	if importRecorder.Code != 200 {
		t.Fatalf("backup restore status=%d body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	query := fmt.Sprintf("SELECT HEX(data), extracted_text, owner FROM ai_agent_attachments WHERE id = %d", attachment.ID)
	args := mysqlClientArgs(mysqlCommand)
	args = append(args[:len(args)-1], "--batch", "--skip-column-names", "--execute", query, targetDatabase)
	output, err := exec.Command(mysqlCommand.execPath, mysqlCommand.fullArgs(args)...).Output()
	if err != nil {
		t.Fatalf("read restored attachment: %v", err)
	}
	columns := strings.SplitN(strings.TrimSpace(string(output)), "\t", 3)
	if len(columns) != 3 || !strings.EqualFold(columns[0], "62696e617279206174746163686d656e74207061796c6f6164") || columns[1] != attachment.ExtractedText || columns[2] != attachment.Owner {
		t.Fatalf("restored attachment did not preserve binary, text, and ownership: %q", output)
	}
}

func makeOfficeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func makeTestPDF(textObject string) []byte {
	content := "BT /F1 12 Tf 72 720 Td " + textObject + " ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buffer bytes.Buffer
	_, _ = io.WriteString(&buffer, "%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for index, object := range objects {
		offsets[index] = buffer.Len()
		_, _ = fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := buffer.Len()
	_, _ = io.WriteString(&buffer, fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objects)+1))
	for _, offset := range offsets {
		_, _ = io.WriteString(&buffer, fmt.Sprintf("%010d 00000 n \n", offset))
	}
	_, _ = io.WriteString(&buffer, fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset))
	return buffer.Bytes()
}
