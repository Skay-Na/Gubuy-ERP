package models

import "time"

// AIProviderConfig stores one provider credential encrypted by the backend.
type AIProviderConfig struct {
	ID           uint      `gorm:"primaryKey" json:"-"`
	Provider     string    `gorm:"type:varchar(32);not null;uniqueIndex" json:"provider"`
	Model        string    `gorm:"type:varchar(100);not null" json:"model"`
	Region       string    `gorm:"type:varchar(16);not null;default:'cn'" json:"region"`
	EncryptedKey string    `gorm:"type:text;not null" json:"-"`
	Active       bool      `gorm:"not null;default:false" json:"active"`
	UpdatedAt    time.Time `json:"updated_at"`
}
