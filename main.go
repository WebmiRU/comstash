package main

import (
	"comstash/internal/models"
	"encoding/json/v2"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
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

func storePackage(pkg *Package) {
	extra, err := json.Marshal(pkg.Extra)
	if err != nil { // @todo удалить эту проверку
		log.Fatal("Ошибка сериализации Extra:", err)
	}

	rec := models.Package{
		Name:              pkg.Name,
		Description:       pkg.Description,
		Keywords:          pq.StringArray(pkg.Keywords),
		Homepage:          pkg.Homepage,
		Version:           pkg.Version,
		VersionNormalized: pkg.VersionNormalized,
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

	// Записываем в БД
	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}, {Name: "version"}},
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
		log.Printf("Ошибка вставки: %v", result.Error)
	}

	//fmt.Printf("Создана запись с ID: %d\n", rec.ID)
}

func main() {
	var err error
	db, err = gorm.Open(postgres.Open("host=localhost user=compo password=compo dbname=compo port=5444 sslmode=disable"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // Отключает добавление "s" к именам таблиц
		},
	})

	if err != nil {
		panic(err)
	}

	j, err := loadPackage("laravel_framework.json")
	if err != nil {
		log.Fatalf("loading laravel_framework.json error: %v", err)
	}

	var description, homepage, _type, supportIssues, supportSource string
	var keywords []string

	for name, pkg := range j.Packages {
		for _, p := range pkg {
			p.Name = name

			if len(p.Description) > 0 {
				description = p.Description
			} else {
				p.Description = description
			}

			if len(p.Homepage) > 0 {
				homepage = p.Homepage
			} else {
				p.Homepage = homepage
			}

			if len(p.Keywords) > 0 {
				keywords = p.Keywords
			} else {
				p.Keywords = keywords
			}

			if len(p.Type) > 0 {
				_type = p.Type
			} else {
				p.Type = _type
			}

			if len(p.Support.Issues) > 0 {
				supportIssues = p.Support.Issues
			} else {
				p.Support.Issues = supportIssues
			}

			if len(p.Support.Source) > 0 {
				supportSource = p.Support.Source
			} else {
				p.Support.Source = supportSource
			}

			//storePackage(&p)
		}
	}

	var pkg models.Package
	// Ищем по имени (Eloquent: Package::where('name', '...')->first())
	db.Where("name = ?", "pack/1").First(&pkg)

	fmt.Printf("Найдено в БД: ID=%d, Name=%s", pkg.ID, pkg.Name)

	port := ":8080"
	fmt.Printf("Сервер слушает на http://localhost%s\n", port)

	http.HandleFunc("/", handler)

	if err := http.ListenAndServe(port, nil); err != nil {
		panic(err)
	}
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

	// 3. Ошибка для всего остального
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `{"error": "not found"}`)
}
