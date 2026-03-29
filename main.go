package main

import (
	"comstash/internal/models"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/lib/pq"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var db *gorm.DB
var packageLoadGroup singleflight.Group

type upstreamError struct {
	StatusCode int
	Err        error
}

func (e *upstreamError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("upstream error, status code: %d", e.StatusCode)
	}

	return fmt.Sprintf("upstream error, status code: %d: %v", e.StatusCode, e.Err)
}

func (e *upstreamError) Unwrap() error {
	return e.Err
}

func requestBaseURL(r *http.Request) string {
	if baseURL := os.Getenv("SERVER_BASE_URL"); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}

	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func ensurePackageData(packageName string) error {
	_, err, _ := packageLoadGroup.Do(packageName, func() (any, error) {
		return nil, getPackageData(packageName)
	})

	return err
}

func isNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func writeHTTPError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case err == nil:
		return
	case isNotFoundError(err):
		http.Error(w, notFoundMsg, http.StatusNotFound)
	case errors.As(err, new(*upstreamError)):
		var upstreamErr *upstreamError
		_ = errors.As(err, &upstreamErr)
		if upstreamErr.StatusCode == http.StatusNotFound {
			http.Error(w, notFoundMsg, http.StatusNotFound)
			return
		}
		log.Printf("upstream error: %v", err)
		http.Error(w, "Upstream package source error", http.StatusBadGateway)
	default:
		log.Printf("internal error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func getPackageData(packageName string) error {
	url := fmt.Sprintf("https://packagist.org/p2/%s.json", packageName) // @todo ENV
	client := &http.Client{Timeout: 20 * time.Second}                   // @todo ENV

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &upstreamError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read upstream response error: %w", err)
	}

	var repo Repository

	err = json.Unmarshal(body, &repo)
	if err != nil {
		return fmt.Errorf("json unmarshal error: %w", err)
	}

	fmt.Printf("Loaded package: %s. Versions count: %d. Updating DB\n", packageName, len(repo.Packages[packageName]))

	if err = db.Transaction(func(tx *gorm.DB) error {
		for name, pkg := range repo.Packages {
			for _, p := range pkg {
				p.Name = name
				if err := storePackage(tx, &p); err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		return err
	}

	fmt.Println("DB successfully updated")

	return nil
}

func storePackage(tx *gorm.DB, pkg *Package) error {
	extra, err := json.Marshal(pkg.Extra)
	if err != nil {
		return fmt.Errorf(`JSON field "extra" serialization error: %w`, err)
	}

	rec := models.Package{
		Name:              pkg.Name,
		Description:       pkg.Description,
		Keywords:          pq.StringArray(pkg.Keywords),
		Homepage:          pkg.Homepage,
		Version:           pkg.Version,
		VersionNormalized: pkg.VersionNormalized,
		License:           nil,
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
			"type",
			"support_issues",
			"support_source",
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

	for _, author := range pkg.Authors {
		a := models.Author{
			Name:      author.Name,
			Email:     author.Email,
			PackageID: rec.ID,
		}

		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "email"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&a).Error

		if err != nil {
			return fmt.Errorf("error while insert author: %w", err)
		}
	}

	for name, version := range pkg.Require {
		require := models.Require{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "version"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&require).Error

		if err != nil {
			return fmt.Errorf("error while insert require: %w", err)
		}
	}

	for name, version := range pkg.RequireDev {
		require := models.RequireDev{
			Name:      name,
			Version:   version,
			PackageID: rec.ID,
		}

		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "name"},
				{Name: "version"},
				{Name: "package_id"},
			},
			DoNothing: true,
		}).Create(&require).Error

		if err != nil {
			return fmt.Errorf("error while insert require-dev: %w", err)
		}
	}

	return nil
}

func main() {
	_ = godotenv.Overload(".env")

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
		panic(err)
	}

	//if err = db.AutoMigrate(
	//	&models.Package{},
	//	&models.Author{},
	//	&models.Require{},
	//	&models.RequireDev{},
	//); err != nil {
	//	log.Fatalf("db automigrate error: %v", err)
	//}

	r := chi.NewRouter()
	//r.Use(middleware.Compress(9, "application/json", "text/xml")) // @todo Make compression level as ENV variable
	r.Get("/packages.json", packages)
	r.Get("/p2/{vendor}/{pkg}.json", vendorPackageHandler)
	r.Get("/cache/{vendor}/{package}", cacheHandler)

	addr := fmt.Sprintf("%s:%s", os.Getenv("SERVER_IP"), os.Getenv("SERVER_PORT"))
	fmt.Println(fmt.Sprintf("Server listening on %s", addr))
	if err = http.ListenAndServe(addr, r); err != nil {
		panic(err)
	}
}

func packages(w http.ResponseWriter, r *http.Request) {
	// @todo Заменить на json struct
	fmt.Fprintf(w, `{"packages":[], "metadata-url":"/p2/%%package%%.json"}`)
}

func getPackageFromDB(packageName string) ([]models.Package, error) {
	var data []models.Package

	result := db.Preload("Authors").
		Preload("Require").
		Preload("RequireDev").
		Where("name = ?", fmt.Sprintf("%s", packageName)).
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

func vendorPackageHandler(w http.ResponseWriter, r *http.Request) {
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "pkg")

	w.Header().Set("Content-Type", "application/json")

	packageName := fmt.Sprintf("%s/%s", vendor, pkg)

	data, err := getPackageFromDB(packageName)
	if err != nil {
		writeHTTPError(w, err, fmt.Sprintf(`Package "%s" not found`, packageName))
		return
	}

	packages := make([]Package, 0, len(data))

	if len(data) == 0 {
		fmt.Printf(`No packages "%s" found in local DB. Loading package data from "packagist.org"\n`, packageName)

		err = ensurePackageData(packageName)
		if err != nil {
			writeHTTPError(w, err, fmt.Sprintf(`Package "%s" not found`, packageName))
			return
		}

		data, err = getPackageFromDB(packageName)
		if err != nil {
			writeHTTPError(w, err, fmt.Sprintf(`Package "%s" not found`, packageName))
			return
		}
	}

	for _, v := range data {
		var extra map[string]any
		if len(v.Extra) > 0 {
			_ = json.Unmarshal(v.Extra, &extra)
		}

		authors := make([]Author, 0, len(v.Authors))
		for _, a := range v.Authors {
			authors = append(authors, Author{
				Name:  a.Name,
				Email: a.Email,
			})
		}

		require := map[string]string{}
		for _, v1 := range v.Require {
			require[v1.Name] = v1.Version
		}

		requireDev := map[string]string{}
		for _, v1 := range v.RequireDev {
			requireDev[v1.Name] = v1.Version
		}

		// Replace original URL for our cache
		if v.DistType == "zip" {
			v.DistUrl = fmt.Sprintf("%s/cache/%s?v=%s", requestBaseURL(r), v.Name, v.Version)
		}

		packages = append(packages, Package{
			Name:              v.Name,
			Description:       v.Description,
			Keywords:          v.Keywords,
			Homepage:          v.Homepage,
			Version:           v.Version,
			VersionNormalized: v.VersionNormalized,
			License:           v.License,
			Type:              v.Type,
			Time:              v.Time,
			Authors:           authors,
			Source: Source{
				URL:       v.SourceUrl,
				Type:      v.SourceType,
				Reference: v.SourceReference,
			},
			Dist: Dist{
				URL:       v.DistUrl,
				Type:      v.DistType,
				Shasum:    v.DistShasum,
				Reference: v.DistReference,
			},
			Support: Support{
				Issues: v.SupportIssues,
				Source: v.SupportSource,
			},
			Funding:    nil,
			Autoload:   nil,
			Extra:      extra,
			Require:    require,
			RequireDev: requireDev,
			Suggest:    nil,
		})
	}

	response := Repository{
		Minified: "composer/2.0",
		Packages: map[string][]Package{
			packageName: packages,
		},
	}

	err = json.MarshalWrite(w, response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func cacheHandler(w http.ResponseWriter, r *http.Request) {
	//w.Header().Set("Content-Type", "application/json")
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "package")
	packageName := fmt.Sprintf("%s/%s", vendor, pkg)
	version := r.URL.Query().Get("v")

	row, err := getPackageMetaFromDB(packageName, version)
	if err != nil {
		if !isNotFoundError(err) {
			writeHTTPError(w, err, fmt.Sprintf(`Package "%s" or version "%s" not found`, packageName, version))
			return
		}

		fmt.Printf("Package %q version %q not found in local DB. Loading metadata from Packagist...\n", packageName, version)

		if err = ensurePackageData(packageName); err != nil {
			writeHTTPError(w, err, fmt.Sprintf(`Package "%s" or version "%s" not found`, packageName, version))
			return
		}

		row, err = getPackageMetaFromDB(packageName, version)
		if err != nil {
			writeHTTPError(w, err, fmt.Sprintf(`Package "%s" or version "%s" not found`, packageName, version))
			return
		}
	}

	switch row.DistType {
	case "zip":
		packageName := fmt.Sprintf("%s/%s", vendor, pkg)
		filepath := fmt.Sprintf("cache/packages/%s/%s.zip", packageName, version)

		exists, err := fileExists(filepath)
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		if !exists {
			fmt.Printf("Package %q not found in cache, downloading...\n", packageName)
			// Create directory for package Cache
			if err = os.MkdirAll(fmt.Sprintf("cache/packages/%s", packageName), 0755); err != nil {
				http.Error(w, "Package cache directory create error", http.StatusInternalServerError)
				return
			}

			if err = downloadFile(row.DistUrl, filepath); err != nil {
				http.Error(w, "Package download error", http.StatusInternalServerError)
				return
			}

			// @todo Calculate sha-hash and update DB packages.shasum column value

			fmt.Printf("Package %q version %q download success\n", packageName, version)
		}

		file, err := os.Open(filepath)
		if err != nil {
			http.Error(w, "Package open error", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Package stat error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))

		_, err = io.Copy(w, file)
		if err != nil {
			log.Printf("File send error: %v", err)
		}

	//case "git":
	//@todo

	default:
		log.Println("Unknown dist type:", row.DistType)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func fileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}
}

func downloadFile(url string, filepath string) error {
	var client = &http.Client{Timeout: 300 * time.Second} // @todo Вынести таймаут в ENV-переменную

	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	tmpPath := filepath + ".part"
	_ = os.Remove(tmpPath)

	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	defer func() {
		out.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	if err = out.Sync(); err != nil {
		return err
	}

	if err = out.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpPath, filepath); err != nil {
		return err
	}

	return nil
}
