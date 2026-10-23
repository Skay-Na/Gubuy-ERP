package models

import "time"

// AIAgentSession holds a private conversation owned by one authenticated user.
type AIAgentSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Owner     string    `gorm:"type:varchar(100);index;not null" json:"-"`
	Debug     bool      `gorm:"not null;default:false;index" json:"debug"`
	Context   string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIAgentMessage struct {
	ID                uint                    `gorm:"primaryKey" json:"id"`
	SessionID         uint                    `gorm:"index;not null" json:"session_id"`
	Role              string                  `gorm:"type:varchar(16);not null" json:"role"`
	Content           string                  `gorm:"type:text;not null" json:"content"`
	Sources           string                  `gorm:"type:text" json:"sources"`
	Evidence          string                  `gorm:"type:mediumtext" json:"evidence,omitempty"`
	Task              string                  `gorm:"type:text" json:"task,omitempty"`
	Trace             string                  `gorm:"type:mediumtext" json:"-"`
	Status            string                  `gorm:"type:varchar(24)" json:"status,omitempty"`
	ActionID          *uint                   `gorm:"index" json:"action_id,omitempty"`
	TaskID            *uint                   `gorm:"index" json:"task_id,omitempty"`
	RunID             string                  `gorm:"type:char(36);index" json:"run_id,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	Attachments       []AIAgentAttachmentInfo `gorm:"-" json:"attachments,omitempty"`
	AttachmentContext string                  `gorm:"-" json:"-"`
}

// AIAgentAttachment keeps uploaded sources with the user message that introduced them.
type AIAgentAttachment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	SessionID     uint      `gorm:"index;not null" json:"-"`
	MessageID     uint      `gorm:"index;not null" json:"message_id"`
	Owner         string    `gorm:"type:varchar(100);index;not null" json:"-"`
	Name          string    `gorm:"type:varchar(255);not null" json:"name"`
	RelativePath  string    `gorm:"type:varchar(1024)" json:"relative_path,omitempty"`
	MediaType     string    `gorm:"type:varchar(120);not null" json:"media_type"`
	Size          int64     `gorm:"not null" json:"size"`
	Data          []byte    `gorm:"type:longblob;not null" json:"-"`
	ExtractedText string    `gorm:"type:longtext" json:"-"`
	Truncated     bool      `gorm:"not null;default:false" json:"truncated,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// AIAgentAttachmentInfo is the metadata exposed by chat history without source bytes.
type AIAgentAttachmentInfo struct {
	ID           uint   `json:"id,omitempty"`
	Name         string `json:"name"`
	RelativePath string `json:"relative_path,omitempty"`
	MediaType    string `json:"media_type"`
	Size         int64  `json:"size"`
	Truncated    bool   `json:"truncated,omitempty"`
}

// AIAgentRun is a compact, structured index for one user request and its trace.
type AIAgentRun struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	RunID                string    `gorm:"type:char(36);uniqueIndex;not null" json:"run_id"`
	Owner                string    `gorm:"type:varchar(100);index;not null" json:"-"`
	SessionID            uint      `gorm:"index;not null" json:"session_id"`
	UserMessageID        uint      `gorm:"index;not null" json:"user_message_id"`
	AssistantMessageID   uint      `gorm:"index;not null" json:"assistant_message_id"`
	Topic                string    `gorm:"type:varchar(40);index" json:"topic"`
	Operation            string    `gorm:"type:varchar(24)" json:"operation"`
	Status               string    `gorm:"type:varchar(24);index;not null" json:"status"`
	FailureCode          string    `gorm:"type:varchar(48);index" json:"failure_code,omitempty"`
	Model                string    `gorm:"type:varchar(100)" json:"model,omitempty"`
	PromptVersion        string    `gorm:"type:varchar(40)" json:"prompt_version,omitempty"`
	ToolVersion          string    `gorm:"type:varchar(40)" json:"tool_version,omitempty"`
	DurationMilliseconds int64     `json:"duration_ms"`
	PromptTokens         int       `json:"prompt_tokens"`
	CompletionTokens     int       `json:"completion_tokens"`
	ToolCalls            int       `json:"tool_calls"`
	EvidenceCount        int       `json:"evidence_count"`
	CreatedAt            time.Time `gorm:"index" json:"created_at"`
}

// AIAgentFeedback stores a user's rating for an assistant message.
type AIAgentFeedback struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	AssistantMessageID uint      `gorm:"uniqueIndex:idx_agent_feedback_message_owner;not null" json:"assistant_message_id"`
	Owner              string    `gorm:"type:varchar(100);uniqueIndex:idx_agent_feedback_message_owner;index;not null" json:"-"`
	Rating             string    `gorm:"type:varchar(12);not null" json:"rating"`
	Category           string    `gorm:"type:varchar(32)" json:"category,omitempty"`
	Comment            string    `gorm:"type:varchar(500)" json:"comment,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// AIAgentCase is a triage item. Its source remains owner-scoped until explicitly redacted.
type AIAgentCase struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	Fingerprint        string    `gorm:"type:char(64);uniqueIndex;not null" json:"-"`
	RunID              string    `gorm:"type:char(36);index;not null" json:"run_id"`
	Owner              string    `gorm:"type:varchar(100);index;not null" json:"-"`
	AssistantMessageID uint      `gorm:"index;not null" json:"assistant_message_id"`
	Detector           string    `gorm:"type:varchar(40);not null" json:"detector"`
	Severity           string    `gorm:"type:varchar(12);not null;default:'normal'" json:"severity"`
	State              string    `gorm:"type:varchar(20);index;not null;default:'new'" json:"state"`
	Category           string    `gorm:"type:varchar(32)" json:"category,omitempty"`
	Comment            string    `gorm:"type:varchar(500)" json:"comment,omitempty"`
	VerifiedLabel      string    `gorm:"type:text" json:"verified_label,omitempty"`
	OccurrenceCount    int       `gorm:"not null;default:1" json:"occurrence_count"`
	CreatedAt          time.Time `gorm:"index" json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// AIAgentPredictionSnapshot preserves a forecast and its data cutoff for later evaluation.
type AIAgentPredictionSnapshot struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	RunID              string     `gorm:"type:char(36);index;not null" json:"run_id"`
	Owner              string     `gorm:"type:varchar(100);index;not null" json:"-"`
	AssistantMessageID uint       `gorm:"index;not null" json:"assistant_message_id"`
	Dataset            string     `gorm:"type:varchar(40);not null" json:"dataset"`
	Metric             string     `gorm:"type:varchar(60);not null" json:"metric"`
	Aggregation        string     `gorm:"type:varchar(12);not null" json:"aggregation"`
	TimeField          string     `gorm:"type:varchar(60);not null" json:"time_field"`
	FiltersJSON        string     `gorm:"type:text" json:"-"`
	TargetMonth        string     `gorm:"type:char(7);index;not null" json:"target_month"`
	AsOf               time.Time  `json:"as_of"`
	MethodVersion      string     `gorm:"type:varchar(40);not null" json:"method_version"`
	ForecastValue      float64    `json:"forecast_value"`
	NaiveValue         float64    `json:"naive_value"`
	ActualValue        *float64   `json:"actual_value,omitempty"`
	EvaluatedAt        *time.Time `json:"evaluated_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// AIAgentKnowledge is shared guidance that only becomes active after review.
type AIAgentKnowledge struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Topic        string     `gorm:"type:varchar(40);index" json:"topic,omitempty"`
	Category     string     `gorm:"type:varchar(32);not null" json:"category"`
	Content      string     `gorm:"type:varchar(2000);not null" json:"content"`
	SourceCaseID *uint      `gorm:"index" json:"source_case_id,omitempty"`
	Status       string     `gorm:"type:varchar(16);index;not null;default:'pending'" json:"status"`
	SubmittedBy  string     `gorm:"type:varchar(100);not null" json:"submitted_by"`
	ReviewedBy   string     `gorm:"type:varchar(100)" json:"reviewed_by,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	ExpiresAt    *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// AIAgentAction is an editable, one-use proposal. Pending actions never affect business data.
type AIAgentAction struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID uint      `gorm:"index;not null" json:"session_id"`
	Owner     string    `gorm:"type:varchar(100);index;not null" json:"-"`
	Kind      string    `gorm:"type:varchar(32);not null" json:"kind"`
	Payload   string    `gorm:"type:text;not null" json:"payload"`
	Status    string    `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	TargetID  *uint     `json:"target_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AIAgentMemory stores user-controlled context across agent sessions.
// Its content is background context, never a business query result.
type AIAgentMemory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Owner     string    `gorm:"type:varchar(100);index;not null" json:"-"`
	Kind      string    `gorm:"type:varchar(20);not null" json:"kind"`
	Content   string    `gorm:"type:varchar(500);not null" json:"content"`
	Source    string    `gorm:"type:varchar(20);not null" json:"source"`
	Evidence  string    `gorm:"type:varchar(500)" json:"evidence,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AIAgentMemoryState prevents deleted memories from being learned again from old conversations.
type AIAgentMemoryState struct {
	Owner        string    `gorm:"type:varchar(100);primaryKey" json:"-"`
	ForgotBefore time.Time `json:"-"`
}
