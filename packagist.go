package main

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

var packageLoadGroup singleflight.Group

type upstreamError struct {
	StatusCode int
	Err        error
}

type unsupportedDistError struct {
	PackageName string
	Version     string
	DistType    string
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

func (e *unsupportedDistError) Error() string {
	if e.DistType == "" {
		return fmt.Sprintf("package %q version %q has no dist metadata", e.PackageName, e.Version)
	}

	return fmt.Sprintf("package %q version %q has unsupported dist type %q", e.PackageName, e.Version, e.DistType)
}

func ensurePackageData(packageName string) error {
	_, err, _ := packageLoadGroup.Do(packageName, func() (any, error) {
		return nil, getPackageData(packageName)
	})

	return err
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
	if err = json.Unmarshal(body, &repo); err != nil {
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
