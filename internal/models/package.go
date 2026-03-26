package models

import "gorm.io/datatypes"

type Package struct {
	ID                uint           `gorm:"primaryKey"`
	Name              string         `gorm:"uniqueIndex"`
	Description       string         `gorm:"type:text"`
	Homepage          string         `gorm:"type:text"`
	Version           string         `gorm:"type:text"`
	VersionNormalized string         `gorm:"type:text"`
	SourceUrl         string         `gorm:"type:text"`
	SourceType        string         `gorm:"type:text"`
	SourceReference   string         `gorm:"type:text"`
	DistUrl           string         `gorm:"type:text"`
	DistType          string         `gorm:"type:text"`
	DistReference     string         `gorm:"type:text"`
	DistShasum        string         `gorm:"type:text"`
	Type              string         `gorm:"type:text"`
	SupportIssues     string         `gorm:"type:text"`
	SupportSource     string         `gorm:"type:text"`
	Time              string         `gorm:"type:text"`
	Extra             datatypes.JSON `gorm:"type:jsonb"`
}
