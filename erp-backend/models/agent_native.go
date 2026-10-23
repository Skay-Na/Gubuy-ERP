package models

import "time"

// AgentSignal is a durable, rule-derived observation. TenantID is fixed to
// "single" until all legacy business tables have real tenant ownership.
type AgentSignal struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TenantID    string    `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	Fingerprint string    `gorm:"type:varchar(120);not null;uniqueIndex" json:"-"`
	SignalType  string    `gorm:"type:varchar(40);not null;index" json:"signal_type"`
	Severity    string    `gorm:"type:varchar(12);not null" json:"severity"`
	ProductID   uint      `gorm:"not null;index" json:"product_id"`
	MetricsJSON string    `gorm:"type:text;not null" json:"metrics"`
	Evidence    string    `gorm:"type:varchar(500);not null" json:"evidence"`
	Status      string    `gorm:"type:varchar(20);not null;default:'open';index" json:"status"`
	DetectedAt  time.Time `gorm:"not null;index" json:"detected_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AgentGoal struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TenantID     string    `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	GoalType     string    `gorm:"type:varchar(40);not null" json:"goal_type"`
	TargetMetric string    `gorm:"type:varchar(40);not null" json:"target_metric"`
	TargetValue  float64   `gorm:"not null" json:"target_value"`
	Deadline     time.Time `gorm:"not null" json:"deadline"`
	Constraints  string    `gorm:"type:varchar(500)" json:"constraints"`
	Priority     int       `gorm:"not null;default:2" json:"priority"`
	Status       string    `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	CreatedBy    string    `gorm:"type:varchar(100);not null" json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AgentWorkTask is independent of a chat session and can be resumed after a restart.
type AgentWorkTask struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	TenantID         string     `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	SignalID         *uint      `gorm:"uniqueIndex" json:"signal_id,omitempty"`
	RelatedSignalID  *uint      `gorm:"index" json:"related_signal_id,omitempty"`
	Kind             string     `gorm:"type:varchar(40);not null;index" json:"kind"`
	Status           string     `gorm:"type:varchar(24);not null;index" json:"status"`
	Step             string     `gorm:"type:varchar(40);not null" json:"step"`
	Attempts         int        `gorm:"not null;default:0" json:"attempts"`
	ErrorCode        string     `gorm:"type:varchar(40)" json:"error_code,omitempty"`
	CreatedBy        string     `gorm:"type:varchar(100);not null;default:'system_monitor';index" json:"created_by"`
	IdempotencyKey   *string    `gorm:"type:varchar(120);uniqueIndex" json:"idempotency_key,omitempty"`
	InputJSON        string     `gorm:"type:text" json:"input,omitempty"`
	CapabilitiesJSON string     `gorm:"type:varchar(500)" json:"-"`
	OutputJSON       string     `gorm:"type:mediumtext" json:"output,omitempty"`
	ChatSessionID    *uint      `gorm:"index" json:"chat_session_id,omitempty"`
	ChatMessageID    *uint      `gorm:"index" json:"chat_message_id,omitempty"`
	LeaseUntil       *time.Time `gorm:"index" json:"-"`
	LeaseVersion     uint64     `gorm:"not null;default:0" json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// AgentPlannerCall stores bounded planner-call telemetry and the validated
// action summary. It intentionally excludes credentials and hidden reasoning.
type AgentPlannerCall struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TaskID           uint      `gorm:"not null;uniqueIndex:ux_agent_planner_call" json:"task_id"`
	CallNumber       int       `gorm:"not null;uniqueIndex:ux_agent_planner_call" json:"call_number"`
	Provider         string    `gorm:"type:varchar(32)" json:"provider,omitempty"`
	Model            string    `gorm:"type:varchar(100)" json:"model,omitempty"`
	DecisionType     string    `gorm:"type:varchar(32);not null" json:"decision_type"`
	ToolName         string    `gorm:"type:varchar(80)" json:"tool_name,omitempty"`
	ReasonSummary    string    `gorm:"type:varchar(500)" json:"reason_summary,omitempty"`
	Status           string    `gorm:"type:varchar(24);not null;index" json:"status"`
	ErrorClass       string    `gorm:"type:varchar(40)" json:"error_class,omitempty"`
	PromptTokens     int       `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completion_tokens"`
	EstimatedTokens  int       `gorm:"not null;default:0" json:"estimated_tokens"`
	TokenUsageKnown  bool      `gorm:"not null;default:false" json:"token_usage_known"`
	LatencyMillis    int64     `gorm:"not null;default:0" json:"latency_millis"`
	CreatedAt        time.Time `json:"created_at"`
}

// AgentSchedule contains a validated recurring business task definition.
// Initially only daily_inventory_check is accepted by the service layer.
type AgentSchedule struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	OwnerID            string     `gorm:"type:varchar(100);not null;uniqueIndex:ux_agent_schedule_owner_task,priority:1" json:"owner_id"`
	IdempotencyKey     *string    `gorm:"type:varchar(140);uniqueIndex" json:"-"`
	TaskType           string     `gorm:"type:varchar(40);not null;uniqueIndex:ux_agent_schedule_owner_task,priority:2" json:"task_type"`
	ParametersJSON     string     `gorm:"type:varchar(1000);not null" json:"parameters"`
	Timezone           string     `gorm:"type:varchar(64);not null" json:"timezone"`
	ScheduleExpression string     `gorm:"type:varchar(80);not null" json:"schedule_expression"`
	NextRunAt          time.Time  `gorm:"not null;index" json:"next_run_at"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	Enabled            bool       `gorm:"not null;default:true;index" json:"enabled"`
	Status             string     `gorm:"type:varchar(24);not null;default:'active';index" json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AgentScheduleRun uniquely identifies one planned occurrence of a schedule.
type AgentScheduleRun struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	ScheduleID      uint       `gorm:"not null;uniqueIndex:ux_agent_schedule_run" json:"schedule_id"`
	PlannedRunAt    time.Time  `gorm:"not null;uniqueIndex:ux_agent_schedule_run" json:"planned_run_at"`
	ActualStartedAt *time.Time `json:"actual_started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	LinkedTaskID    *uint      `gorm:"index" json:"linked_task_id,omitempty"`
	Status          string     `gorm:"type:varchar(24);not null;index" json:"status"`
	Error           string     `gorm:"type:varchar(500)" json:"error,omitempty"`
	RetryCount      int        `gorm:"not null;default:0" json:"retry_count"`
	LeaseUntil      *time.Time `gorm:"index" json:"-"`
	LeaseVersion    uint64     `gorm:"not null;default:0" json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AgentWorkTaskStep stores one executable tool invocation and its durable outcome.
type AgentWorkTaskStep struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TaskID         uint       `gorm:"not null;uniqueIndex:ux_agent_task_step" json:"task_id"`
	Sequence       int        `gorm:"not null;uniqueIndex:ux_agent_task_step" json:"sequence"`
	StepType       string     `gorm:"type:varchar(32);not null" json:"step_type"`
	ToolName       string     `gorm:"type:varchar(80);not null" json:"tool_name"`
	InputSummary   string     `gorm:"type:varchar(500);not null" json:"input_summary"`
	Status         string     `gorm:"type:varchar(24);not null;index" json:"status"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	ResultSummary  string     `gorm:"type:text" json:"result_summary,omitempty"`
	RetryCount     int        `gorm:"not null;default:0" json:"retry_count"`
	ErrorMessage   string     `gorm:"type:varchar(500)" json:"error_message,omitempty"`
	IdempotencyKey string     `gorm:"type:varchar(140);not null;uniqueIndex" json:"idempotency_key"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AgentToolCallAudit records every invocation through the shared executor.
type AgentToolCallAudit struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TaskID         *uint      `gorm:"index" json:"task_id,omitempty"`
	CallID         string     `gorm:"type:varchar(100);not null;uniqueIndex" json:"call_id"`
	IdempotencyKey string     `gorm:"type:varchar(140);index" json:"idempotency_key"`
	TenantID       string     `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	UserID         string     `gorm:"type:varchar(100)" json:"user_id,omitempty"`
	Principal      string     `gorm:"type:varchar(32);not null" json:"principal"`
	ToolName       string     `gorm:"type:varchar(80);not null;index" json:"tool_name"`
	InputSummary   string     `gorm:"type:varchar(500);not null" json:"input_summary"`
	Status         string     `gorm:"type:varchar(24);not null;index" json:"status"`
	OutputSummary  string     `gorm:"type:varchar(500)" json:"output_summary,omitempty"`
	ErrorMessage   string     `gorm:"type:varchar(500)" json:"error_message,omitempty"`
	StartedAt      time.Time  `gorm:"not null" json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// AgentReplenishmentDraft is an internal suggestion, not a purchase order.
type AgentReplenishmentDraft struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	TenantID      string     `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	TaskID        uint       `gorm:"uniqueIndex;not null" json:"task_id"`
	ProductID     uint       `gorm:"not null;index" json:"product_id"`
	Quantity      int        `gorm:"not null" json:"quantity"`
	StockSnapshot int        `gorm:"not null" json:"stock_snapshot"`
	Sales30Days   int        `gorm:"not null" json:"sales_30_days"`
	CostSnapshot  float64    `gorm:"type:decimal(10,2);not null;default:0" json:"cost_snapshot"`
	DataNote      string     `gorm:"type:varchar(500);not null" json:"data_note"`
	Status        string     `gorm:"type:varchar(24);not null;default:'suggested'" json:"status"`
	ReviewedBy    string     `gorm:"type:varchar(100)" json:"reviewed_by,omitempty"`
	ReviewNote    string     `gorm:"type:varchar(500)" json:"review_note,omitempty"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type AgentActivity struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  string    `gorm:"type:varchar(40);not null;index" json:"tenant_id"`
	TaskID    uint      `gorm:"index;not null" json:"task_id"`
	Event     string    `gorm:"type:varchar(40);not null" json:"event"`
	Detail    string    `gorm:"type:varchar(500);not null" json:"detail"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
