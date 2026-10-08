package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erp-backend/config"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type aiOrderItem struct {
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type aiImportFields struct {
	CustomerName    string        `json:"customer_name,omitempty"`
	CustomerPhone   string        `json:"customer_phone,omitempty"`
	CustomerAddress string        `json:"customer_address,omitempty"`
	EmployeeName    string        `json:"employee_name,omitempty"`
	Items           []aiOrderItem `json:"items,omitempty"`
	Amount          float64       `json:"amount,omitempty"`
	PaymentChannel  string        `json:"payment_channel,omitempty"`
	IsSubsidy       bool          `json:"is_subsidy,omitempty"`
	Category        string        `json:"category,omitempty"`
	Counterparty    string        `json:"counterparty,omitempty"`
	OccurredAt      string        `json:"occurred_at,omitempty"`
	Remark          string        `json:"remark,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

// AnalyzeAIImport recognizes an image and stores a reviewable draft.
// @Summary 识别订单或支出图片
// @Tags AI Import
// @Accept multipart/form-data
// @Produce json
// @Param kind formData string true "order 或 expense"
// @Param image formData file true "图片"
// @Description 可选 formData 字段 description 用于补充图片中难以辨认的信息。
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID post_api_ai_imports
// @Router /api/ai/imports [post]
func AnalyzeAIImport(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 9<<20)
	kind := c.PostForm("kind")
	if kind != "order" && kind != "expense" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择订单或支出"})
		return
	}
	_, isEmployee, scopeErr := currentEmployeeScope(c)
	if scopeErr != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return
	}
	if isEmployee && kind != "order" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "员工仅可拍照录入订单"})
		return
	}
	runtime, err := activeAIProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": err.Error()})
		return
	}
	file, err := c.FormFile("image")
	if err != nil || file.Size > 8<<20 || file.Size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择不超过 8MB 的图片"})
		return
	}
	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "图片读取失败"})
		return
	}
	defer reader.Close()
	image, err := io.ReadAll(io.LimitReader(reader, 8<<20+1))
	if err != nil || len(image) > 8<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "图片过大"})
		return
	}
	mime := http.DetectContentType(image)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "仅支持 JPG、PNG、WebP 图片"})
		return
	}
	hashBytes := sha256.Sum256(image)
	hash := hex.EncodeToString(hashBytes[:])
	var previous models.AIImport
	if err := config.DB.Select("id", "owner", "status").Where("kind = ? AND image_hash = ?", kind, hash).First(&previous).Error; err == nil {
		if previous.Owner == c.GetString("username") || (previous.Owner == "" && !isEmployee) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "这张图片已导入，请打开已有草稿或记录", "data": gin.H{"id": previous.ID, "status": previous.Status}})
		} else {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "这张图片已导入，请使用另一张图片"})
		}
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "重复图片检查失败"})
		return
	}
	fields, err := extractAIFields(c.Request.Context(), runtime, kind, c.PostForm("description"), mime, image)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "图片识别失败：" + err.Error()})
		return
	}
	extracted, _ := json.Marshal(fields)
	draft := models.AIImport{Kind: kind, Owner: c.GetString("username"), ImageHash: hash, ImageType: mime, Image: image, Extracted: string(extracted), Status: "pending"}
	if err := config.DB.Create(&draft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "草稿保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"id": draft.ID, "fields": fields}})
}

func scopedAIImports(c *gin.Context) (*gorm.DB, bool) {
	_, isEmployee, err := currentEmployeeScope(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
		return nil, false
	}
	query := config.DB.Model(&models.AIImport{})
	if isEmployee {
		return query.Where("owner = ? AND kind = ?", c.GetString("username"), "order"), true
	}
	return query.Where("(owner = ? OR owner = '' OR owner IS NULL)", c.GetString("username")), true
}

func extractAIFields(ctx context.Context, runtime aiProviderRuntime, kind, description, mime string, image []byte) (aiImportFields, error) {
	var fields aiImportFields
	prompt := "只从图片和用户描述提取可辨认的信息，返回 JSON 对象。看不清的字段留空或 0，不要猜测。warnings 列出含糊字段。"
	if kind == "order" {
		prompt += "字段：customer_name, customer_phone, customer_address, employee_name（经手人/开单员工姓名）, amount（订单合计金额）, payment_channel（图片中出现的支付渠道原文，例如云闪付）, is_subsidy（出现云闪付时必须为 true）, items（数组，每项 name, quantity, unit_price）, warnings。商品名称和型号保留原文；即使手写型号省略了品牌、品类或部分字符，也要提取所有可辨认的型号字符。即使商品行没有填写单价，也必须提取合计栏金额到 amount，不要把合计金额丢弃。"
	} else {
		prompt += "这是支出截图。字段：amount（实际支出正数）, category, counterparty, occurred_at（YYYY-MM-DD，可为空）, remark, warnings。分类优先从渠道奖励、员工提成、安装费、日常采购、运输配送、水电物业、员工报销、营销推广、房租、其他支出中选择。"
	}
	requestBody := gin.H{
		"model": runtime.Model,
		"messages": []gin.H{
			{"role": "system", "content": prompt},
			{"role": "user", "content": []gin.H{
				{"type": "text", "text": "用户描述：" + strings.TrimSpace(description)},
				{"type": "image_url", "image_url": gin.H{"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)}},
			}},
		},
	}
	if runtime.JSONMode {
		requestBody["response_format"] = gin.H{"type": "json_object"}
	}
	if runtime.Provider == "qwen" {
		requestBody["enable_thinking"] = false
	}
	body, _ := json.Marshal(requestBody)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, runtime.URL, bytes.NewReader(body))
	if err != nil {
		return fields, err
	}
	request.Header.Set("Authorization", "Bearer "+runtime.APIKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fields, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fields, fmt.Errorf("模型服务返回 %d", response.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return fields, err
	}
	if len(result.Choices) == 0 {
		return fields, errors.New("模型未返回识别结果")
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```JSON")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(strings.TrimSpace(content), "```")
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &fields) != nil {
		return fields, errors.New("模型未返回有效 JSON")
	}
	normalizeAIImportFields(&fields, content)
	return fields, nil
}

func normalizeAIImportFields(fields *aiImportFields, rawModelOutput string) {
	// 云闪付是本系统国补订单的确定性标志。这里在模型输出之后再次兜底，
	// 避免模型正确识别出文字却漏填 is_subsidy。
	if !strings.Contains(fields.PaymentChannel, "云闪付") && !strings.Contains(rawModelOutput, "云闪付") {
		return
	}
	fields.IsSubsidy = true
	if fields.PaymentChannel == "" {
		fields.PaymentChannel = "云闪付"
	}
}

// GetAIImport returns a draft or completed import without its image bytes.
// @Summary 查看 AI 导入记录
// @Tags AI Import
// @Produce json
// @Param id path int true "导入 ID"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID get_api_ai_imports_id
// @Router /api/ai/imports/{id} [get]
func GetAIImport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效草稿编号"})
		return
	}
	var draft models.AIImport
	query, ok := scopedAIImports(c)
	if !ok {
		return
	}
	if err := query.Select("id", "kind", "extracted", "status", "target_id", "created_at").First(&draft, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "草稿不存在"})
		return
	}
	var fields aiImportFields
	_ = json.Unmarshal([]byte(draft.Extracted), &fields)
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"id": draft.ID, "kind": draft.Kind, "status": draft.Status, "target_id": draft.TargetID, "fields": fields}})
}

// ListAIImports lists recent drafts and completed records without loading image blobs.
// @Summary 最近 AI 导入记录
// @Tags AI Import
// @Produce json
// @Param kind query string true "order 或 expense"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID get_api_ai_imports
// @Router /api/ai/imports [get]
func ListAIImports(c *gin.Context) {
	kind := c.Query("kind")
	if kind != "order" && kind != "expense" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效导入类型"})
		return
	}
	query, ok := scopedAIImports(c)
	if !ok {
		return
	}
	if strings.HasPrefix(c.GetString("username"), "emp_") && kind != "order" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "员工仅可查看自己的订单草稿"})
		return
	}
	var imports []models.AIImport
	if err := query.Select("id", "kind", "status", "target_id", "created_at").
		Where("kind = ?", kind).Order("id DESC").Limit(30).Find(&imports).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "导入记录查询失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": imports})
}

// GetAIImportImage returns the protected original image.
// @Summary 获取 AI 导入原图
// @Tags AI Import
// @Produce image/jpeg
// @Param id path int true "导入 ID"
// @Success 200 {file} file
// @Security ApiKeyAuth
// @ID get_api_ai_imports_id_image
// @Router /api/ai/imports/{id}/image [get]
func GetAIImportImage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	var draft models.AIImport
	query, ok := scopedAIImports(c)
	if !ok {
		return
	}
	if err := query.Select("image", "image_type").First(&draft, id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, draft.ImageType, draft.Image)
}

// DeleteAIImport removes an owned import draft and its stored source image.
// Business records previously created from the draft are intentionally preserved.
// @Summary 删除 AI 导入草稿
// @Tags AI Import
// @Param id path int true "导入 ID"
// @Success 200 {object} APIResponse
// @Security ApiKeyAuth
// @ID delete_api_ai_imports_id
// @Router /api/ai/imports/{id} [delete]
func DeleteAIImport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效草稿编号"})
		return
	}

	query, ok := scopedAIImports(c)
	if !ok {
		return
	}
	result := query.Where("id = ?", uint(id)).Delete(&models.AIImport{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "草稿删除失败"})
		return
	}
	if result.RowsAffected != 1 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "草稿不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "草稿及原图已删除"})
}

func lockAIImport(tx *gorm.DB, id uint, kind, owner string) error {
	if id == 0 {
		return nil
	}
	var draft models.AIImport
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "kind", "owner", "status").First(&draft, id).Error; err != nil {
		return errors.New("AI 草稿不存在")
	}
	if draft.Kind != kind || draft.Status != "pending" {
		return errors.New("AI 草稿已使用或类型不匹配")
	}
	if draft.Owner != owner && !(draft.Owner == "" && !strings.HasPrefix(owner, "emp_")) {
		return errors.New("无权使用此 AI 草稿")
	}
	return nil
}

func completeAIImport(tx *gorm.DB, id uint, targetID uint) error {
	if id == 0 {
		return nil
	}
	result := tx.Model(&models.AIImport{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]interface{}{"status": "saved", "target_id": targetID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("AI 草稿已使用")
	}
	return nil
}
