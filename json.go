package main

import "encoding/json/v2"

type Repository struct {
	Minified string               `json:"minified"`
	Packages map[string][]Package `json:"packages"`
}

type Package struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Keywords          []string  `json:"keywords"`
	Homepage          string    `json:"homepage"`
	Version           string    `json:"version"`
	VersionNormalized string    `json:"version_normalized"`
	License           []string  `json:"license"`
	Type              string    `json:"type"`
	Time              string    `json:"time"`
	Authors           []Author  `json:"authors"`
	Source            Source    `json:"source"`
	Dist              Dist      `json:"dist"`
	Support           Support   `json:"support"`
	Funding           any       `json:"funding"`
	Autoload          any       `json:"autoload"`
	Extra             any       `json:"extra"`
	Require           StringMap `json:"require"`
	RequireDev        StringMap `json:"require-dev"`
	Suggest           any       `json:"suggest,omitzero"` // map[string]string || string
}

type StringMap map[string]string

func (m *StringMap) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*m = nil
		return nil
	}

	var value map[string]string
	if err := json.Unmarshal(data, &value); err == nil {
		*m = value
		return nil
	}

	// Packagist sometimes returns "__unset" instead of an object.
	var ignored any
	if err := json.Unmarshal(data, &ignored); err != nil {
		return err
	}

	*m = nil
	return nil
}

type Support struct {
	Issues string `json:"issues"`
	Source string `json:"source"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Source struct {
	URL       string `json:"url"`
	Type      string `json:"type"`
	Reference string `json:"reference"`
}

type Dist struct {
	URL       string `json:"url"`
	Type      string `json:"type"`
	Shasum    string `json:"shasum"`
	Reference string `json:"reference"`
}
