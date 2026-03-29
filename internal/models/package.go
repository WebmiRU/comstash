package models

import "gorm.io/datatypes"

type Package struct {
	ID                uint           `gorm:"primaryKey"`
	Name              string         `gorm:"uniqueIndex:idx_package_name_version;index;type:text COLLATE NOCASE"`
	Description       string         `gorm:"type:text"`
	Keywords          datatypes.JSON `gorm:"column:keywords;type:JSON"`
	Homepage          string         `gorm:"type:text"`
	Version           string         `gorm:"type:text;uniqueIndex:idx_package_name_version"`
	VersionNormalized string         `gorm:"type:text"`
	License           datatypes.JSON `gorm:"column:license;type:JSON"`
	Authors           []Author       `gorm:"foreignKey:PackageID"`
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
	Extra             datatypes.JSON `gorm:"type:JSON"`
	Require           []Require      `gorm:"foreignKey:PackageID"`
	RequireDev        []RequireDev   `gorm:"foreignKey:PackageID"`
}
