//go:build integration

package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"
	"github.com/gin-gonic/gin"
)

func TestGetAIAgentMessageTraceOwnership(t *testing.T) {
	if err := middlewares.ConfigureJWTSecret("isolated-test-signing-secret-32-bytes-minimum"); err != nil {
		t.Fatal(err)
	}
	session := models.AIAgentSession{Owner: "trace_test_owner"}
	if err := config.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := config.DB.Delete(&models.AIAgentMessage{}, "session_id = ?", session.ID).Error; err != nil {
			t.Error(err)
		}
		if err := config.DB.Delete(&session).Error; err != nil {
			t.Error(err)
		}
	})
	message := models.AIAgentMessage{
		SessionID: session.ID, Role: "assistant", Content: "answer",
		Trace: `{"stop_reason":"done"}`,
	}
	if err := config.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/trace/:message_id", middlewares.AdminAuthRequired(), GetAIAgentMessageTrace)
	tests := []struct {
		name, user, id string
		want           int
	}{
		{name: "owner", user: session.Owner, id: strconv.Itoa(int(message.ID)), want: 200},
		{name: "another admin", user: "another_admin", id: strconv.Itoa(int(message.ID)), want: 404},
		{name: "employee", user: "emp_1", id: strconv.Itoa(int(message.ID)), want: 403},
		{name: "unauthenticated", id: strconv.Itoa(int(message.ID)), want: 401},
		{name: "invalid id", user: session.Owner, id: "bad", want: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/trace/"+tt.id, nil)
			if tt.user != "" {
				token, err := middlewares.GenerateToken(tt.user)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if tt.want == 200 {
				var result struct {
					Data struct {
						Trace map[string]any `json:"trace"`
					} `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.Trace["stop_reason"] != "done" {
					t.Fatalf("trace=%v", result.Data.Trace)
				}
			}
		})
	}
}
