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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

var db *gorm.DB

func loadPackage(filename string) (*Repository, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file error: %w", err)
	}

	var repo Repository

	// Опция RejectUnknownMembers(true) выдаст ошибку, если в JSON есть поля, которых нет в структуре (полезно для строгой валидации)
	err = json.Unmarshal(data, &repo)
	if err != nil {
		return nil, fmt.Errorf("json v2 unmarshal error: %w", err)
	}

	return &repo, nil
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
		return errors.New(fmt.Sprintf("server response error, status code: %d", resp.StatusCode))
	}

	body, _ := io.ReadAll(resp.Body)

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
				storePackage(tx, &p)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	fmt.Println("DB successfully updated")

	return nil
}

func storePackage(tx *gorm.DB, pkg *Package) {
	extra, err := json.Marshal(pkg.Extra)
	if err != nil {
		log.Fatal(`json field "extra" serialization error:`, err)
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
		log.Printf(`error while insert "package" row: %v`, result.Error)
		return
	}

	if rec.ID == 0 {
		tx.Where("name = ? AND version = ?", pkg.Name, pkg.Version).First(&rec)
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
			log.Printf("Ошибка вставки автора: %v", err)
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
			log.Printf("DB insert error: %v", err)
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
			log.Printf("DB insert error: %v", err)
		}
	}
}

func main() {
	_ = godotenv.Overload(".env")

	var err error
	db, err = gorm.Open(sqlite.Open("db"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
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

	j, err := loadPackage("laravel_framework.json")
	if err != nil {
		log.Fatalf("loading laravel_framework.json error: %v", err)
	}

	if err = db.Transaction(func(tx *gorm.DB) error {
		for name, pkg := range j.Packages {
			for _, p := range pkg {
				p.Name = name
				storePackage(tx, &p)
			}
		}

		return nil
	}); err != nil {
		log.Fatalf("package import transaction error: %v", err)
	}

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

func vendorPackageHandler(w http.ResponseWriter, r *http.Request) {
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "pkg")

	w.Header().Set("Content-Type", "application/json")

	packageName := fmt.Sprintf("%s/%s", vendor, pkg)

	data, err := getPackageFromDB(packageName)
	if err != nil {
		log.Println(err)
	}

	packages := make([]Package, 0, len(data))

	if len(data) == 0 {
		fmt.Printf(`No packages "%s" found in local DB. Loading package data from "packagist.org"\n`, packageName)

		err = getPackageData(packageName)
		if err != nil {
			log.Println(err) // @todo Возможно не хватает какой-то доп. обработки ошибок
		}

		data, err = getPackageFromDB(packageName)
		if err != nil {
			log.Println(err)
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
			v.DistUrl = fmt.Sprintf("http://localhost:8080/cache/%s?v=%s", v.Name, v.Version) // @todo Change DOMAIN and PORT
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

	fmt.Println(vendor, pkg, version)

	var row models.Package
	result := db.Preload("Authors").
		Preload("Require").
		Where("name = ? AND version = ?", fmt.Sprintf("%s/%s", vendor, pkg), version).
		First(&row)

	if result.Error != nil {
		log.Println(result.Error)
	}

	if row.ID == 0 {
		http.Error(w, fmt.Sprintf(`Package "%s" or version "%s" not found`, packageName, version), http.StatusNotFound)
		return
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
			os.MkdirAll(fmt.Sprintf("cache/packages/%s", packageName), 0755)

			if err = downloadFile(row.DistUrl, filepath); err != nil {
				http.Error(w, "Package download error", http.StatusInternalServerError)
				return
			}

			// @todo Calculate sha-hash and update DB packages.shasum column value

			fmt.Printf("Package %q %q download success\n", packageName, version)
		}

		file, _ := os.Open(filepath)
		defer file.Close()

		stat, _ := file.Stat()
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

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}

	defer out.Close()

	_, err = io.Copy(out, resp.Body)

	return err
}
