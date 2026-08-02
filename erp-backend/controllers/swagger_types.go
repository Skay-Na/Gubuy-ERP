package controllers

// APIResponse documents the common JSON envelope returned by the HTTP API.
// Data is intentionally open because each operation returns a different payload.
type APIResponse struct {
	Code        int         `json:"code" example:"200"`
	Msg         string      `json:"msg,omitempty"`
	Data        interface{} `json:"data,omitempty"`
	Total       int64       `json:"total,omitempty"`
	Initialized bool        `json:"initialized,omitempty"`
}
