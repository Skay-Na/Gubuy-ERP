package models

import "time"

// AIImport keeps the source image together with the reviewed business record.
type AIImport struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Kind      string    `gorm:"type:varchar(20);not null;uniqueIndex:idx_ai_import_image" json:"kind"`
	Owner     string    `gorm:"type:varchar(100);index" json:"-"`
	ImageHash string    `gorm:"type:char(64);not null;uniqueIndex:idx_ai_import_image" json:"-"`
	ImageType string    `gorm:"type:varchar(30);not null" json:"image_type"`
	Image     []byte    `gorm:"type:longblob" json:"-"`
	Extracted string    `gorm:"type:longtext" json:"extracted"`
	Status    string    `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	TargetID  *uint     `gorm:"index" json:"target_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
