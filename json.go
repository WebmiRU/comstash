package main

import "encoding/json/v2"

type Repository struct {
	Minified           string               `json:"minified,omitzero"`
	Packages           map[string][]Package `json:"packages"`
	SecurityAdvisories []any                `json:"security-advisories"`
}

type Package struct {
	Name              string    `json:"name,omitzero"`
	Description       string    `json:"description,omitzero"`
	Keywords          []string  `json:"keywords,omitzero"`
	Homepage          string    `json:"homepage,omitzero"`
	Version           string    `json:"version"`
	VersionNormalized string    `json:"version_normalized"`
	License           []string  `json:"license,omitzero"`
	Type              string    `json:"type,omitzero"`
	Time              string    `json:"time,omitzero"`
	Authors           []Author  `json:"authors,omitzero"`
	Source            *Source   `json:"source,omitzero"`
	Dist              *Dist     `json:"dist,omitzero"`
	Support           Support   `json:"support,omitzero"`
	Funding           any       `json:"funding,omitzero"`
	Autoload          any       `json:"autoload,omitzero"`
	Extra             any       `json:"extra,omitzero"`
	Require           StringMap `json:"require,omitzero"`
	RequireDev        StringMap `json:"require-dev,omitzero"`
	Suggest           any       `json:"suggest,omitzero"` // map[string]string || string

	RequireUnset    bool `json:"-"`
	RequireDevUnset bool `json:"-"`
	FundingUnset    bool `json:"-"`
	AutoloadUnset   bool `json:"-"`
	ExtraUnset      bool `json:"-"`
	SuggestUnset    bool `json:"-"`
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

func (p *Package) UnmarshalJSON(data []byte) error {
	type packageAlias Package
	type packageDecode struct {
		packageAlias
		Require    jsonField `json:"require"`
		RequireDev jsonField `json:"require-dev"`
		Funding    jsonField `json:"funding"`
		Autoload   jsonField `json:"autoload"`
		Extra      jsonField `json:"extra"`
		Suggest    jsonField `json:"suggest"`
	}

	var decoded packageDecode
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*p = Package(decoded.packageAlias)

	if decoded.Require.isUnset() {
		p.Require = nil
		p.RequireUnset = true
	}
	if decoded.RequireDev.isUnset() {
		p.RequireDev = nil
		p.RequireDevUnset = true
	}
	if decoded.Funding.isUnset() {
		p.Funding = nil
		p.FundingUnset = true
	}
	if decoded.Autoload.isUnset() {
		p.Autoload = nil
		p.AutoloadUnset = true
	}
	if decoded.Extra.isUnset() {
		p.Extra = nil
		p.ExtraUnset = true
	}
	if decoded.Suggest.isUnset() {
		p.Suggest = nil
		p.SuggestUnset = true
	}

	return nil
}

type jsonField []byte

func (f *jsonField) UnmarshalJSON(data []byte) error {
	*f = append((*f)[:0], data...)
	return nil
}

func (f jsonField) isUnset() bool {
	return string(f) == `"__unset"`
}

type Support struct {
	Issues string `json:"issues,omitzero"`
	Source string `json:"source,omitzero"`
}

type Author struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitzero"`
	Homepage string `json:"homepage,omitzero"`
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

func newSource(url, sourceType, reference string) *Source {
	if url == "" && sourceType == "" && reference == "" {
		return nil
	}

	return &Source{
		URL:       url,
		Type:      sourceType,
		Reference: reference,
	}
}

func newDist(url, distType, shasum, reference string) *Dist {
	if url == "" && distType == "" && shasum == "" && reference == "" {
		return nil
	}

	return &Dist{
		URL:       url,
		Type:      distType,
		Shasum:    shasum,
		Reference: reference,
	}
}
