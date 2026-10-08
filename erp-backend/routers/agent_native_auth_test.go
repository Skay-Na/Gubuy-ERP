package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"erp-backend/middlewares"
)

func TestAgentManagerEndpointsRequireAdmin(t *testing.T) {
	if err := middlewares.ConfigureJWTSecret("isolated-test-signing-secret-32-bytes-minimum"); err != nil {
		t.Fatal(err)
	}
	router := SetupRouter()
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/ai/manager/signals"},
		{http.MethodGet, "/api/ai/manager/tasks"},
		{http.MethodGet, "/api/ai/manager/drafts"},
		{http.MethodPost, "/api/ai/manager/drafts/1/review"},
		{http.MethodGet, "/api/ai/manager/goals"},
		{http.MethodGet, "/api/ai/manager/activities"},
		{http.MethodPost, "/api/ai/manager/goals"},
		{http.MethodPost, "/api/ai/manager/scan"},
	}
	employeeToken, err := middlewares.GenerateToken("emp_123")
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			unauthenticated := httptest.NewRecorder()
			router.ServeHTTP(unauthenticated, request)
			if unauthenticated.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
			}
			employeeRequest := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			employeeRequest.Header.Set("Authorization", "Bearer "+employeeToken)
			employee := httptest.NewRecorder()
			router.ServeHTTP(employee, employeeRequest)
			if employee.Code != http.StatusForbidden {
				t.Fatalf("employee status = %d", employee.Code)
			}
		})
	}
}
