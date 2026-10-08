package controllers

import (
	"bytes"
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestEmployeePhotoOrderIsPrivateAndOneUse(t *testing.T) {
	setupAudit(t)
	if err := config.DB.Where("1 = 1").Delete(&models.AIImport{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Where("1 = 1").Delete(&models.AIProviderConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	first := models.Employee{Name: "照片员工甲", EmpNo: "photo-one-" + suffix}
	second := models.Employee{Name: "照片员工乙", EmpNo: "photo-two-" + suffix}
	if err := config.DB.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := config.DB.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	firstUser := "emp_" + first.EmpNo
	secondUser := "emp_" + second.EmpNo
	productID := ensureProduct(t, "照片录单测试商品", 100, 60, 10)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		content, _ := json.Marshal(gin.H{"customer_name": "测试客户", "customer_phone": "13800000000", "items": []gin.H{{"name": "照片录单测试商品", "quantity": 1, "unit_price": 100}}})
		_ = json.NewEncoder(w).Encode(gin.H{"choices": []gin.H{{"message": gin.H{"content": string(content)}}}})
	}))
	defer model.Close()
	t.Setenv("AI_API_KEY", "photo-test-key")
	t.Setenv("AI_MODEL", "photo-test-model")
	t.Setenv("AI_API_URL", model.URL)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("username", c.GetHeader("X-Test-User")) })
	router.POST("/imports", AnalyzeAIImport)
	router.GET("/imports", ListAIImports)
	router.GET("/imports/:id", GetAIImport)
	router.GET("/imports/:id/image", GetAIImportImage)
	router.DELETE("/imports/:id", DeleteAIImport)
	router.POST("/orders", CreateOrder)
	call := func(username, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		request.Header.Set("X-Test-User", username)
		request.Header.Set("Content-Type", contentType)
		router.ServeHTTP(recorder, request)
		return recorder
	}
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	_ = form.WriteField("kind", "order")
	image, err := form.CreateFormFile("image", "order.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = image.Write([]byte("\x89PNG\r\n\x1a\nphoto-order-test"))
	_ = form.Close()
	response := call(firstUser, http.MethodPost, "/imports", form.FormDataContentType(), upload.Bytes())
	if response.Code != 200 {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.Data.ID == 0 {
		t.Fatalf("draft id: %v %s", err, response.Body.String())
	}
	id := strconv.FormatUint(uint64(created.Data.ID), 10)
	if result := call(firstUser, http.MethodGet, "/imports/"+id, "", nil); result.Code != 200 {
		t.Fatalf("own draft: %d %s", result.Code, result.Body.String())
	}
	if result := call(secondUser, http.MethodGet, "/imports/"+id, "", nil); result.Code != 404 {
		t.Fatalf("other employee accessed draft: %d", result.Code)
	}
	if result := call(secondUser, http.MethodGet, "/imports/"+id+"/image", "", nil); result.Code != 404 {
		t.Fatalf("other employee accessed image: %d", result.Code)
	}
	if result := call(secondUser, http.MethodDelete, "/imports/"+id, "", nil); result.Code != 404 {
		t.Fatalf("other employee deleted draft: %d", result.Code)
	}
	if result := call(secondUser, http.MethodGet, "/imports?kind=order", "", nil); result.Code != 200 || strings.Contains(result.Body.String(), `"id":`+id) {
		t.Fatalf("other employee listing: %d %s", result.Code, result.Body.String())
	}
	if result := call(firstUser, http.MethodGet, "/imports?kind=expense", "", nil); result.Code != 403 {
		t.Fatalf("employee expense listing: %d", result.Code)
	}
	orderBody, _ := json.Marshal(gin.H{"ai_import_id": created.Data.ID, "items": []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 100}}, "customer_name": "测试客户", "payment_method": 1, "delivery_method": 2})
	if result := call(secondUser, http.MethodPost, "/orders", "application/json", orderBody); result.Code != 409 {
		t.Fatalf("other employee used draft: %d %s", result.Code, result.Body.String())
	}
	if result := call(firstUser, http.MethodPost, "/orders", "application/json", orderBody); result.Code != 200 {
		t.Fatalf("own order: %d %s", result.Code, result.Body.String())
	}
	if result := call(firstUser, http.MethodPost, "/orders", "application/json", orderBody); result.Code != 409 {
		t.Fatalf("repeat order: %d %s", result.Code, result.Body.String())
	}
	var order models.Order
	if err := config.DB.Where("customer_name = ?", "测试客户").First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.EmployeeID == nil || *order.EmployeeID != first.ID {
		t.Fatalf("order employee = %v, want %d", order.EmployeeID, first.ID)
	}
	if result := call(firstUser, http.MethodDelete, "/imports/"+id, "", nil); result.Code != 200 {
		t.Fatalf("delete own saved import: %d %s", result.Code, result.Body.String())
	}
	var importCount int64
	if err := config.DB.Model(&models.AIImport{}).Where("id = ?", created.Data.ID).Count(&importCount).Error; err != nil || importCount != 0 {
		t.Fatalf("deleted import count = %d, err = %v", importCount, err)
	}
	if err := config.DB.First(&order, order.ID).Error; err != nil {
		t.Fatalf("generated order was removed with import: %v", err)
	}
}
