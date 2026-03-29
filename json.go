package main

type Repository struct {
	Minified string               `json:"minified"`
	Packages map[string][]Package `json:"packages"`
}

type Package struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Keywords          []string          `json:"keywords"`
	Homepage          string            `json:"homepage"`
	Version           string            `json:"version"`
	VersionNormalized string            `json:"version_normalized"`
	License           []string          `json:"license"`
	Type              string            `json:"type"`
	Time              string            `json:"time"`
	Authors           []Author          `json:"authors"`
	Source            Source            `json:"source"`
	Dist              Dist              `json:"dist"`
	Support           Support           `json:"support"`
	Funding           any               `json:"funding"`
	Autoload          Autoload          `json:"autoload"`
	Extra             map[string]any    `json:"extra"`
	Require           map[string]string `json:"require"`
	RequireDev        map[string]string `json:"require-dev"`
	Suggest           any               `json:"suggest,omitzero"` // map[string]string || string
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

type Autoload struct {
	Files []string       `json:"files,omitempty"`
	Psr4  map[string]any `json:"psr-4,omitempty"` // any, так как значение может быть строкой или массивом строк
}
