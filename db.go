package main

import (
	"comstash/internal/models"
	"encoding/json/v2"
	"fmt"
	"log"
	"os"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var db *gorm.DB

func initDB() error {
	var err error
	db, err = gorm.Open(sqlite.Open("db"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger: gormlogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			gormlogger.Config{
				LogLevel:                  gormlogger.Warn,
				IgnoreRecordNotFoundError: true,
			},
		),
	})
	if err != nil {
		return err
	}

	if err = configureSQLite(); err != nil {
		return err
	}

	return migrateSchema()
}

func configureSQLite() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}

	for _, query := range pragmas {
		if err := db.Exec(query).Error; err != nil {
			return fmt.Errorf("sqlite pragma error for %q: %w", query, err)
		}
	}

	return nil
}

func migrateSchema() error {
	if !db.Migrator().HasTable(&models.Package{}) {
		if err := db.AutoMigrate(&models.Package{}); err != nil {
			return err
		}
	}

	if !db.Migrator().HasTable(&models.Author{}) {
		if err := db.AutoMigrate(&models.Author{}); err != nil {
			return err
		}
	}

	if !db.Migrator().HasTable(&models.Require{}) {
		if err := db.AutoMigrate(&models.Require{}); err != nil {
			return err
		}
	}

	if !db.Migrator().HasTable(&models.RequireDev{}) {
		if err := db.AutoMigrate(&models.RequireDev{}); err != nil {
			return err
		}
	}

	for _, column := range []string{"Funding", "Autoload", "Suggest"} {
		if !db.Migrator().HasColumn(&models.Package{}, column) {
			if err := db.Migrator().AddColumn(&models.Package{}, column); err != nil {
				return err
			}
		}
	}

	if !db.Migrator().HasColumn(&models.Author{}, "Homepage") {
		if err := db.Migrator().AddColumn(&models.Author{}, "Homepage"); err != nil {
			return err
		}
	}

	return nil
}

func marshalStringSlice(values []string) (datatypes.JSON, error) {
	if len(values) == 0 {
		return []byte("[]"), nil
	}

	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func unmarshalStringSlice(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		log.Printf("string slice JSON unmarshal error: %v", err)
		return nil
	}

	if len(values) == 0 {
		return nil
	}

	return values
}

func marshalJSONField(value any) (datatypes.JSON, error) {
	if value == nil {
		return nil, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func unmarshalJSONField(data datatypes.JSON) any {
	if len(data) == 0 {
		return nil
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		log.Printf("json field unmarshal error: %v", err)
		return nil
	}

	return value
}

func storePackage(tx *gorm.DB, pkg *Package) error {
	keywords, err := marshalStringSlice(pkg.Keywords)
	if err != nil {
		return fmt.Errorf(`JSON field "keywords" serialization error: %w`, err)
	}

	license, err := marshalStringSlice(pkg.License)
	if err != nil {
		return fmt.Errorf(`JSON field "license" serialization error: %w`, err)
	}

	extra, err := marshalJSONField(pkg.Extra)
	if err != nil {
		return fmt.Errorf(`JSON field "extra" serialization error: %w`, err)
	}

	funding, err := marshalJSONField(pkg.Funding)
	if err != nil {
		return fmt.Errorf(`JSON field "funding" serialization error: %w`, err)
	}

	autoload, err := marshalJSONField(pkg.Autoload)
	if err != nil {
		return fmt.Errorf(`JSON field "autoload" serialization error: %w`, err)
	}

	suggest, err := marshalJSONField(pkg.Suggest)
	if err != nil {
		return fmt.Errorf(`JSON field "suggest" serialization error: %w`, err)
	}

	rec := models.Package{
		Name:              pkg.Name,
		Description:       pkg.Description,
		Keywords:          keywords,
		Homepage:          pkg.Homepage,
		Version:           pkg.Version,
		VersionNormalized: pkg.VersionNormalized,
		License:           license,
		Authors:           nil,
		SourceUrl:         pkg.Source.URL,
		SourceType:        pkg.Source.Type,
		SourceReference:   pkg.Source.Reference,
		DistUrl:           pkg.Dist.URL,
		DistType:          pkg.Dist.Type,
		DistReference:     pkg.Dist.Reference,
		DistShasum:        pkg.Dist.Shasum,
		Type:              pkg.Type,
		SupportIssues:     pkg.Support.Issues,
		SupportSource:     pkg.Support.Source,
		Time:              pkg.Time,
		Extra:             datatypes.JSON(extra),
		Funding:           datatypes.JSON(funding),
		Autoload:          datatypes.JSON(autoload),
		Suggest:           datatypes.JSON(suggest),
	}

	result := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "name"},
			{Name: "version"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"updated_at",
			"description",
			"keywords",
			"homepage",
			"version_normalized",
			"license",
			"source_url",
			"source_type",
			"source_reference",
			"dist_url",
			"dist_type",
			"dist_reference",
			"dist_shasum",
			"type",
			"support_issues",
			"support_source",
			"time",
			"extra",
			"funding",
			"autoload",
			"suggest",
		}),
	}).Create(&rec)

	if result.Error != nil {
		return fmt.Errorf(`error while insert "package" row: %w`, result.Error)
	}

	if rec.ID == 0 {
		if err = tx.Where("name = ? AND version = ?", pkg.Name, pkg.Version).First(&rec).Error; err != nil {
			return fmt.Errorf(`error while load inserted "package" row: %w`, err)
		}
	}

	if err = tx.Where("package_id = ?", rec.ID).Delete(&models.Author{}).Error; err != nil {
		return fmt.Errorf("error while cleanup authors: %w", err)
	}

	if err = tx.Where("package_id = ?", rec.ID).Delete(&models.Require{}).Error; err != nil {
		return fmt.Errorf("error while cleanup require: %w", err)
	}

	if err = tx.Where("package_id = ?", rec.ID).Delete(&models.RequireDev{}).Error; err != nil {
		return fmt.Errorf("error while cleanup require-dev: %w", err)
	}

	for _, author := range pkg.Authors {
		a := models.Author{
			Name:      author.Name,
			Email:     author.Email,
			Homepage:  author.Homepage,
			PackageID: rec.ID,
		}

		if err = tx.Create(&a).Error; err != nil {
			return fmt.Errorf("error while insert author: %w", err)
		}
	}

	for name, version := range pkg.Require {
		require := models.Require{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		if err = tx.Create(&require).Error; err != nil {
			return fmt.Errorf("error while insert require: %w", err)
		}
	}

	for name, version := range pkg.RequireDev {
		require := models.RequireDev{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		if err = tx.Create(&require).Error; err != nil {
			return fmt.Errorf("error while insert require-dev: %w", err)
		}
	}

	return nil
}

func getPackageFromDB(packageName string) ([]models.Package, error) {
	var data []models.Package

	result := db.Preload("Authors").
		Preload("Require").
		Preload("RequireDev").
		Where("name = ?", packageName).
		Find(&data)

	if result.Error != nil {
		return nil, result.Error
	}

	return data, nil
}

func getPackageVersionFromDB(packageName string, version string) (*models.Package, error) {
	var data models.Package

	result := db.Preload("Authors").
		Preload("Require").
		Preload("RequireDev").
		Where("name = ? AND version = ?", packageName, version).
		First(&data)

	if result.Error != nil {
		return nil, result.Error
	}

	return &data, nil
}

func getPackageMetaFromDB(packageName string, version string) (*models.Package, error) {
	var data models.Package

	result := db.Where("name = ? AND version = ?", packageName, version).First(&data)
	if result.Error != nil {
		return nil, result.Error
	}

	return &data, nil
}
