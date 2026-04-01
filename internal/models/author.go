package models

type Author struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"type:text"`
	Email     string `gorm:"type:text"`
	Homepage  string `gorm:"type:text"`
	PackageID uint   `gorm:"index"`
}
