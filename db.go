package main

import (
	"comstash/internal/models"
	"encoding/json/v2"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var db *gorm.DB

func initDB() error {
	var err error
	db, err = gorm.Open(openDBDialector(), &gorm.Config{
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

	return configureDB()
}

func openDBDialector() gorm.Dialector {
	driver := os.Getenv("DB_DRIVER")
	dsn := os.Getenv("DB_DSN")

	switch driver {
	case "", "sqlite", "sqlite3":
		if dsn == "" {
			dsn = "db"
		}
		return sqlite.Open(dsn)
	case "postgres", "postgresql":
		if dsn == "" {
			dsn = "host=127.0.0.1 user=postgres password=postgres dbname=comstash port=5432 sslmode=disable"
		}
		return postgres.Open(dsn)
	default:
		panic(fmt.Sprintf("unsupported DB_DRIVER %q", driver))
	}
}

func configureDB() error {
	switch os.Getenv("DB_DRIVER") {
	case "", "sqlite", "sqlite3":
		return configureSQLite()
	default:
		return nil
	}
}

func configureSQLite() error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("sqlite db handle error: %w", err)
	}

	// SQLite handles concurrent reads well in WAL mode, but writes still serialize.
	// Keep a single pooled connection so every operation shares the same pragmas and
	// we do not hit "database is locked" under concurrent package refreshes.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxIdleTime(0)
	sqlDB.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=10000;",
	}

	for _, query := range pragmas {
		if err = db.Exec(query).Error; err != nil {
			return fmt.Errorf("sqlite pragma error for %q: %w", query, err)
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
	return unmarshalStringSliceWithOption(data, false)
}

func unmarshalStringSlicePreserveEmpty(data datatypes.JSON) []string {
	return unmarshalStringSliceWithOption(data, true)
}

func unmarshalStringSliceWithOption(data datatypes.JSON, preserveEmpty bool) []string {
	if len(data) == 0 {
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		log.Printf("string slice JSON unmarshal error: %v", err)
		return nil
	}

	if len(values) == 0 && !preserveEmpty {
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
		SourceUrl:         sourceValue(pkg.Source, func(v *Source) string { return v.URL }),
		SourceType:        sourceValue(pkg.Source, func(v *Source) string { return v.Type }),
		SourceReference:   sourceValue(pkg.Source, func(v *Source) string { return v.Reference }),
		DistUrl:           sourceValue(pkg.Dist, func(v *Dist) string { return v.URL }),
		DistType:          sourceValue(pkg.Dist, func(v *Dist) string { return v.Type }),
		DistReference:     sourceValue(pkg.Dist, func(v *Dist) string { return v.Reference }),
		DistShasum:        sourceValue(pkg.Dist, func(v *Dist) string { return v.Shasum }),
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

		if err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "email"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&a).Error; err != nil {
			return fmt.Errorf("error while insert author: %w", err)
		}
	}

	for name, version := range pkg.Require {
		require := models.Require{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		if err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "version"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&require).Error; err != nil {
			return fmt.Errorf("error while insert require: %w", err)
		}
	}

	for name, version := range pkg.RequireDev {
		require := models.RequireDev{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		if err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "version"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&require).Error; err != nil {
			return fmt.Errorf("error while insert require-dev: %w", err)
		}
	}

	return nil
}

func sourceValue[T any](value *T, getter func(*T) string) string {
	if value == nil {
		return ""
	}

	return getter(value)
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

func getPackageLastFetchedAt(packageName string) (*time.Time, error) {
	var row models.PackageCacheTTL

	result := db.Where("package_name = ?", packageName).First(&row)
	if result.Error != nil {
		return nil, result.Error
	}

	return &row.LastFetchedAt, nil
}

func updatePackageLastFetchedAt(tx *gorm.DB, packageName string, fetchedAt time.Time) error {
	row := models.PackageCacheTTL{
		PackageName:   packageName,
		LastFetchedAt: fetchedAt.UTC(),
	}

	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "package_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_fetched_at",
		}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf(`error while upsert "package_cache_ttl" row: %w`, err)
	}

	return nil
}

func packageCacheTTL() (time.Duration, bool, error) {
	raw := strings.TrimSpace(os.Getenv("PACKAGIST_CACHE_TTL"))
	if raw == "" {
		return 0, false, nil
	}

	value := strings.Fields(raw)[0]
	minutes, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid PACKAGIST_CACHE_TTL %q: %w", raw, err)
	}

	if minutes <= 0 {
		return 0, false, nil
	}

	return time.Duration(minutes) * time.Minute, true, nil
}
