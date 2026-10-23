package models

import "time"

// OperationHistory stores the before/after rows for a reversible business request.
// Changes are kept server-side and are never returned by the history API.
type OperationHistory struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Username  string     `gorm:"type:varchar(100);not null;index" json:"username"`
	Title     string     `gorm:"type:varchar(160);not null" json:"title"`
	Method    string     `gorm:"type:varchar(10);not null" json:"method"`
	Path      string     `gorm:"type:varchar(255);not null" json:"path"`
	Changes   string     `gorm:"type:longtext;not null" json:"-"`
	UndoneAt  *time.Time `json:"undone_at,omitempty"`
	UndoneBy  string     `gorm:"type:varchar(100)" json:"undone_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
