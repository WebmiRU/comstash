package main

import (
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
	"gorm.io/gorm"
)

func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/packages.json", packages)
	r.Get("/p2/{vendor}/{pkg}.json", vendorPackageHandler)
	r.Get("/cache/{vendor}/{package}", cacheHandler)
	return r
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

func isNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func writeHTTPError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case err == nil:
		return
	case isNotFoundError(err):
		http.Error(w, notFoundMsg, http.StatusNotFound)
	case errors.As(err, new(*unsupportedDistError)):
		http.Error(w, "Package dist is unavailable", http.StatusNotFound)
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

func packages(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `{"packages":[], "metadata-url":"/p2/%%package%%.json"}`)
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
		var authors []Author
		for _, a := range v.Authors {
			authors = append(authors, Author{
				Name:     a.Name,
				Email:    a.Email,
				Homepage: a.Homepage,
			})
		}

		var require map[string]string
		for _, v1 := range v.Require {
			if require == nil {
				require = map[string]string{}
			}
			require[v1.Name] = v1.Version
		}

		var requireDev map[string]string
		for _, v1 := range v.RequireDev {
			if requireDev == nil {
				requireDev = map[string]string{}
			}
			requireDev[v1.Name] = v1.Version
		}

		pkg := Package{
			Version:           v.Version,
			VersionNormalized: v.VersionNormalized,
			Time:              v.Time,
			Authors:           authors,
			Source:            newSource(v.SourceUrl, v.SourceType, v.SourceReference),
			Dist:              newDist(v.DistUrl, v.DistType, v.DistShasum, v.DistReference),
			Support: Support{
				Issues: v.SupportIssues,
				Source: v.SupportSource,
			},
			Funding:    unmarshalJSONField(v.Funding),
			Autoload:   unmarshalJSONField(v.Autoload),
			Extra:      unmarshalJSONField(v.Extra),
			Require:    require,
			RequireDev: requireDev,
			Suggest:    unmarshalJSONField(v.Suggest),
		}

		pkg.Name = v.Name
		pkg.Description = v.Description
		pkg.Keywords = unmarshalStringSlicePreserveEmpty(v.Keywords)
		pkg.Homepage = v.Homepage
		pkg.License = unmarshalStringSlice(v.License)
		pkg.Type = v.Type

		packages = append(packages, pkg)
	}

	for i := range packages {
		if packages[i].Dist != nil && packages[i].Dist.Type == "zip" {
			packages[i].Dist.URL = fmt.Sprintf("%s/cache/%s?v=%s", requestBaseURL(r), packageName, data[i].Version)
		}
	}

	response := Repository{
		SecurityAdvisories: []any{},
		Packages: map[string][]Package{
			packageName: packages,
		},
	}

	if err = json.MarshalWrite(w, response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func cacheHandler(w http.ResponseWriter, r *http.Request) {
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

	if row.DistType == "" || row.DistUrl == "" {
		fmt.Printf("Package %q version %q has incomplete dist metadata. Refreshing from Packagist...\n", packageName, version)
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
		filepath := fmt.Sprintf("cache/packages/%s/%s.zip", packageName, version)

		exists, err := fileExists(filepath)
		if err != nil {
			http.Error(w, "Package cache lookup error", http.StatusInternalServerError)
			return
		}

		if !exists {
			fmt.Printf("Package %q not found in cache, downloading...\n", packageName)
			if err = os.MkdirAll(fmt.Sprintf("cache/packages/%s", packageName), 0755); err != nil {
				http.Error(w, "Package cache directory create error", http.StatusInternalServerError)
				return
			}

			if err = downloadFile(row.DistUrl, filepath); err != nil {
				http.Error(w, "Package download error", http.StatusInternalServerError)
				return
			}

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

		if _, err = io.Copy(w, file); err != nil {
			log.Printf("File send error: %v", err)
		}
	default:
		writeHTTPError(w, &unsupportedDistError{
			PackageName: packageName,
			Version:     version,
			DistType:    row.DistType,
		}, fmt.Sprintf(`Package "%s" or version "%s" not found`, packageName, version))
	}
}

func fileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func downloadFile(url string, filepath string) error {
	client := &http.Client{Timeout: 300 * time.Second} // @todo ENV

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
		_ = out.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = io.Copy(out, resp.Body); err != nil {
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
