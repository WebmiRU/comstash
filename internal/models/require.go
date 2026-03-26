package models

type Require struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"type:text"`
	Version   string `gorm:"type:text"`
	PackageID uint   `gorm:"index"`
}
