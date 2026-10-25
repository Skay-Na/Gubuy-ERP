package routers

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

type swaggerParameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}

type swaggerOperation struct {
	OperationID string             `json:"operationId"`
	Parameters  []swaggerParameter `json:"parameters"`
}

type swaggerDocument struct {
	BasePath string                                 `json:"basePath"`
	Paths    map[string]map[string]swaggerOperation `json:"paths"`
}

var swaggerPathParameter = regexp.MustCompile(`\{([^}]+)\}`)

func TestSwaggerMatchesRegisteredAPIRoutes(t *testing.T) {
	raw, err := os.ReadFile("../docs/swagger.json")
	if err != nil {
		t.Fatalf("read generated swagger document: %v", err)
	}

	var document swaggerDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse generated swagger document: %v", err)
	}
	if document.BasePath != "/" {
		t.Fatalf("basePath must be / because documented paths already include /api; got %q", document.BasePath)
	}

	registered := make(map[string]bool)
	for _, route := range SetupRouter().Routes() {
		if strings.HasPrefix(route.Path, "/api/") {
			// 跳过已停用功能的通配兜底路由（deprecation stub，非 API 契约）：
			// /api/ai/manager/*path 用 Any 注册占位 404，swag 无法也无需为其生成文档。
			if strings.Contains(route.Path, "/*") {
				continue
			}
			registered[strings.ToUpper(route.Method)+" "+route.Path] = true
		}
	}

	documented := make(map[string]bool)
	operationIDs := make(map[string]string)
	for path, methods := range document.Paths {
		if strings.Contains(path, ":") {
			t.Errorf("Swagger path must use {name}, not Gin :name syntax: %s", path)
		}
		ginPath := swaggerPathParameter.ReplaceAllString(path, `:$1`)

		for method, operation := range methods {
			key := strings.ToUpper(method) + " " + ginPath
			documented[key] = true
			if operation.OperationID == "" {
				t.Errorf("operationId is missing for %s", key)
			} else if previous, exists := operationIDs[operation.OperationID]; exists {
				t.Errorf("duplicate operationId %q on %s and %s", operation.OperationID, previous, key)
			} else {
				operationIDs[operation.OperationID] = key
			}

			for _, parameter := range operation.Parameters {
				if (parameter.In == "body" || parameter.In == "path" || parameter.In == "formData") && !parameter.Required {
					t.Errorf("%s parameter %q must be required for %s", parameter.In, parameter.Name, key)
				}
			}
			for _, match := range swaggerPathParameter.FindAllStringSubmatch(path, -1) {
				found := false
				for _, parameter := range operation.Parameters {
					if parameter.In == "path" && parameter.Name == match[1] && parameter.Required {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("required path parameter %q is missing for %s", match[1], key)
				}
			}
		}
	}

	for route := range registered {
		if !documented[route] {
			t.Errorf("registered API route is missing from Swagger: %s", route)
		}
	}
	for route := range documented {
		if !registered[route] {
			t.Errorf("Swagger contains a route that is not registered: %s", route)
		}
	}
}
