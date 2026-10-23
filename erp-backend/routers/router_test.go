package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"erp-backend/middlewares"
)

func TestSystemBackupEndpointsRequireAdmin(t *testing.T) {
	if err := middlewares.ConfigureJWTSecret("isolated-backup-test-secret-at-least-32-bytes"); err != nil {
		t.Fatal(err)
	}
	employeeToken, err := middlewares.GenerateToken("emp_123")
	if err != nil {
		t.Fatal(err)
	}
	router := SetupRouter()
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "export", method: http.MethodGet, path: "/api/system/backup"},
		{name: "restore", method: http.MethodPost, path: "/api/system/restore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			unauthenticated := httptest.NewRecorder()
			router.ServeHTTP(unauthenticated, request)
			if unauthenticated.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
			}
			request = httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set("Authorization", "Bearer "+employeeToken)
			employee := httptest.NewRecorder()
			router.ServeHTTP(employee, request)
			if employee.Code != http.StatusForbidden {
				t.Fatalf("employee status = %d, want 403", employee.Code)
			}
		})
	}
}
