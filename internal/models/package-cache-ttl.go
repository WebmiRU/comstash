package models

import "time"

type PackageCacheTTL struct {
	ID            uint      `gorm:"primaryKey"`
	PackageName   string    `gorm:"uniqueIndex;index;type:text COLLATE NOCASE"`
	LastFetchedAt time.Time `gorm:"type:datetime"`
}
