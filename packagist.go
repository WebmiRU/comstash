package main

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"reflect"
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
		expandPackages := repo.Minified == "composer/2.0"

		for name, pkg := range repo.Packages {
			versions := pkg
			if expandPackages {
				versions = expandMinifiedPackages(pkg)
			}

			for _, p := range versions {
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

func expandMinifiedPackages(packages []Package) []Package {
	if len(packages) == 0 {
		return nil
	}

	expanded := make([]Package, 0, len(packages))
	var previous Package

	for i, pkg := range packages {
		if i == 0 {
			expanded = append(expanded, pkg)
			previous = pkg
			continue
		}

		current := previous
		current.Version = pkg.Version
		current.VersionNormalized = pkg.VersionNormalized

		if pkg.Name != "" {
			current.Name = pkg.Name
		}
		if pkg.Description != "" {
			current.Description = pkg.Description
		}
		if pkg.Keywords != nil {
			current.Keywords = pkg.Keywords
		}
		if pkg.Homepage != "" {
			current.Homepage = pkg.Homepage
		}
		if pkg.License != nil {
			current.License = pkg.License
		}
		if pkg.Type != "" {
			current.Type = pkg.Type
		}
		if pkg.Time != "" {
			current.Time = pkg.Time
		}
		if pkg.Authors != nil {
			current.Authors = pkg.Authors
		}
		if pkg.Source != nil {
			current.Source = pkg.Source
		}
		if pkg.Dist != nil {
			current.Dist = pkg.Dist
		}
		if !reflect.DeepEqual(pkg.Support, Support{}) {
			current.Support = pkg.Support
		}
		if pkg.Funding != nil {
			current.Funding = pkg.Funding
		} else if pkg.FundingUnset {
			current.Funding = nil
		}
		if pkg.Autoload != nil {
			current.Autoload = pkg.Autoload
		} else if pkg.AutoloadUnset {
			current.Autoload = nil
		}
		if pkg.Extra != nil {
			current.Extra = pkg.Extra
		} else if pkg.ExtraUnset {
			current.Extra = nil
		}
		if pkg.Require != nil {
			current.Require = pkg.Require
		} else if pkg.RequireUnset {
			current.Require = nil
		}
		if pkg.RequireDev != nil {
			current.RequireDev = pkg.RequireDev
		} else if pkg.RequireDevUnset {
			current.RequireDev = nil
		}
		if pkg.Suggest != nil {
			current.Suggest = pkg.Suggest
		} else if pkg.SuggestUnset {
			current.Suggest = nil
		}

		expanded = append(expanded, current)
		previous = current
	}

	return expanded
}
