package main

import (
	"comstash/internal/models"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/go-chi/chi/v5"
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

func storePackage(pkg *Package) {
	extra, err := json.Marshal(pkg.Extra)
	if err != nil {
		log.Fatal(`JSON field "extra" serialization error:`, err)
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

	result := db.Clauses(clause.OnConflict{
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
		log.Printf(`Error while insert "package" row: %v`, result.Error)
		return
	}

	if rec.ID == 0 {
		db.Where("name = ? AND version = ?", pkg.Name, pkg.Version).First(&rec)
	}

	for _, author := range pkg.Authors {
		a := models.Author{
			Name:      author.Name,
			Email:     author.Email,
			PackageID: rec.ID,
		}

		err := db.Clauses(clause.OnConflict{
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
}

func main() {
	var err error
	db, err = gorm.Open(postgres.Open("host=localhost user=compo password=compo dbname=compo port=5444 sslmode=disable"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})

	if err != nil {
		panic(err)
	}

	j, err := loadPackage("laravel_framework.json")
	if err != nil {
		log.Fatalf("loading laravel_framework.json error: %v", err)
	}

	for name, pkg := range j.Packages {
		for _, p := range pkg {
			p.Name = name

			//storePackage(&p)
		}
	}

	var pkg models.Package
	db.Where("name = ?", "pack/1").First(&pkg)

	fmt.Printf("Найдено в БД: ID=%d, Name=%s", pkg.ID, pkg.Name)

	r := chi.NewRouter()
	//r.Use(middleware.Compress(9, "application/json", "text/xml"))
	r.Get("/packages.json", packages)
	r.Get("/p2/{vendor}/{pkg}.json", vendorPackageHandler)
	r.Get("/cache/{vendor}/{package}", cacheHandler)

	if err = http.ListenAndServe("0.0.0.0:8080", r); err != nil {
		panic(err)
	}
}

func packages(w http.ResponseWriter, r *http.Request) {
	// @todo Заменить на json struct
	fmt.Fprintf(w, `{"packages":[], "metadata-url":"/p2/%%package%%.json"}`)
}

func vendorPackageHandler(w http.ResponseWriter, r *http.Request) {
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "pkg")

	w.Header().Set("Content-Type", "application/json")

	var data []models.Package
	result := db.Preload("Authors").Where("name = ?", fmt.Sprintf("%s/%s", vendor, pkg)).Find(&data)

	if result.Error != nil {
		log.Println(result.Error)
	}

	fmt.Printf("Найдено версий: %d\n", len(data))

	packages := make([]Package, 0, len(data))

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
			Funding: nil,
			Autoload: Autoload{
				Files: nil,
				Psr4:  nil,
			},
			Extra:      extra,
			Require:    nil,
			RequireDev: nil,
			Suggest:    nil,
		})
	}

	packageName := fmt.Sprintf("%s/%s", vendor, pkg)
	response := Repository{
		Minified: "composer/2.0",
		Packages: map[string][]Package{
			packageName: packages,
		},
	}

	err := json.MarshalWrite(w, response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func cacheHandler(w http.ResponseWriter, r *http.Request) {
	//w.Header().Set("Content-Type", "application/json")
	vendor := chi.URLParam(r, "vendor")
	pkg := chi.URLParam(r, "package")
	version := r.URL.Query().Get("v")

	fmt.Println(vendor, pkg, version)

	var row models.Package
	result := db.Preload("Authors").Where("name = ? AND version = ?", fmt.Sprintf("%s/%s", vendor, pkg), version).First(&row)

	if result.Error != nil {
		log.Println(result.Error)
	}

	if row.ID == 0 {
		http.Error(w, "Package or version not found", http.StatusNotFound)
		return
	}

	switch row.DistType {
	case "zip":
		filepath := fmt.Sprintf("cache/packages/%s/%s/%s.zip", vendor, pkg, version)
		fmt.Println("FILENAME:", filepath)
		exists, err := fileExists(filepath)
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		if !exists {
			if err = downloadFile(row.DistUrl, filepath); err != nil {
				http.Error(w, "Package download error", http.StatusInternalServerError)
				return
			}

			fmt.Printf("Package download success: %s/%s %s\n", vendor, pkg, version)
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

func handler(w http.ResponseWriter, r *http.Request) {

	// --- ОТЛАДКА В КОНСОЛЬ ---
	// Выводим весь запрос: метод, заголовки, путь
	dump, _ := httputil.DumpRequest(r, false)
	fmt.Printf("\n--- ВХОДЯЩИЙ ЗАПРОС ---\n%s\n", string(dump))

	path := r.URL.Path
	w.Header().Set("Content-Type", "application/json")

	// --- ЛОГИКА РЕПОЗИТОРИЯ ---

	// 1. Точка входа
	if path == "/packages.json" {
		fmt.Fprintf(w, `{"packages":[], "metadata-url":"/p2/%%package%%.json"}`)
		return
	}

	// 2. Запрос метаданных пакета (V2 протокол)
	if strings.HasPrefix(path, "/p2/") && strings.HasSuffix(path, ".json") {
		// Извлекаем "vendor/package" из "/p2/vendor/package.json"
		packageName := strings.TrimSuffix(strings.TrimPrefix(path, "/p2/"), ".json")

		fmt.Printf(">>> Composer ищет метаданные для: %s\n", packageName)

		// Пример динамического ответа для любого пакета
		// В реальной жизни здесь будет поиск ZIP-файла в папке
		fmt.Fprintf(w, `{
			"packages": {
				"%s": [
					{
						"name": "%s",
						"version": "1.0.0",
						"dist": {
							"type": "zip",
							"url": "http://localhost:8080/dist/%s-1.0.0.zip",
							"reference": "v1.0.0"
						},
						"require": { "php": "^8.0" }
					}
				]
			}
		}`, packageName, packageName, strings.ReplaceAll(packageName, "/", "-"))
		return
	}

	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `{"error": "not found"}`)
}
