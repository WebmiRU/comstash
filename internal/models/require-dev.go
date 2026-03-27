package models

type RequireDev struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"type:text"`
	Version   string `gorm:"type:text"`
	PackageID uint   `gorm:"index"`
}
