// Package skillcatalog contains the I/O-independent skill catalog contract.
package skillcatalog

import "encoding/json"

type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Extensions    map[string]any    `json:"extensions,omitempty"`
	AllowedTools  []string          `json:"allowed_tools,omitempty"`
	Body          string            `json:"-"`
	Dir           string            `json:"-"`
	Source        string            `json:"source"`
	Digest        string            `json:"digest"`
	PackageBytes  int64             `json:"-"`
}

type Issue struct {
	Name   string `json:"name,omitempty"`
	Source string `json:"source"`
	Error  string `json:"error"`
}

type Catalog struct {
	Skills []Skill `json:"skills"`
	Issues []Issue `json:"issues,omitempty"`
	Digest string  `json:"digest"`
}

func (c Catalog) JSON() ([]byte, error) { return json.Marshal(c) }

func (c Catalog) Find(name string) (Skill, bool) {
	for _, skill := range c.Skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return Skill{}, false
}
