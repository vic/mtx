// Package theme holds the color schemes mtx can use: every base16, base24 and tinted8 scheme of
// github.com/tinted-theming/schemes (MIT, see schemes/LICENSE), embedded.
package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed schemes
var files embed.FS

// Scheme is one color scheme as 16 base16 slots ("base00".."base0F", "#rrggbb"); base24 schemes keep
// their extra slots too.
type Scheme struct {
	ID      string // "base16/gruvbox-dark-hard": system and file name, unique
	Name    string
	System  string // base16, base24 or tinted8
	Variant string // dark or light
	Colors  map[string]string
}

// file is the YAML of one scheme. base16 and base24 files keep their colors under palette with base
// slot names; tinted8 files name them by color and say their family and style under scheme.
type file struct {
	System  string            `yaml:"system"`
	Name    string            `yaml:"name"`
	Variant string            `yaml:"variant"`
	Palette map[string]string `yaml:"palette"`
	Scheme  struct {
		Family string `yaml:"family"`
		Style  string `yaml:"style"`
	} `yaml:"scheme"`
}

// tinted8Slots maps each base16 slot to the tinted8 color that fills it.
var tinted8Slots = [16][2]string{
	{"base00", "black"}, {"base01", "black-bright"}, {"base02", "black-bright"}, {"base03", "gray"},
	{"base04", "gray"}, {"base05", "white"}, {"base06", "white"}, {"base07", "white"},
	{"base08", "red"}, {"base09", "orange"}, {"base0A", "yellow"}, {"base0B", "green"},
	{"base0C", "cyan"}, {"base0D", "blue"}, {"base0E", "magenta"}, {"base0F", "cyan-dim"},
}

// tinted8Fallback is the color for a tinted8 name the file leaves out: cyan-dim is cyan, black-bright
// is gray, else black.
func tinted8Fallback(palette map[string]string, name string) (string, bool) {
	switch name {
	case "cyan-dim":
		c, ok := palette["cyan"]
		return c, ok
	case "black-bright":
		if c, ok := palette["gray"]; ok {
			return c, true
		}
		c, ok := palette["black"]
		return c, ok
	}
	return "", false
}

var (
	once    sync.Once
	schemes []Scheme
	byID    map[string]Scheme
	errs    []error
)

// All returns every scheme that parsed, sorted by ID, and the errors of the files that did not.
func All() ([]Scheme, []error) {
	load()
	return slices.Clone(schemes), slices.Clone(errs)
}

// ByID returns the scheme with the given ID, such as "base16/gruvbox-dark-hard".
func ByID(id string) (Scheme, bool) {
	load()
	s, ok := byID[id]
	return s, ok
}

// load parses the embedded files once.
func load() {
	once.Do(func() {
		byID = map[string]Scheme{}
		err := fs.WalkDir(files, "schemes", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !(strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
				return nil
			}
			s, err := parse(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p, err))
				return nil
			}
			schemes = append(schemes, s)
			byID[s.ID] = s
			return nil
		})
		if err != nil {
			errs = append(errs, err)
		}
		slices.SortFunc(schemes, func(a, b Scheme) int { return strings.Compare(a.ID, b.ID) })
	})
}

// parse reads the scheme file at p (inside files) into a Scheme.
func parse(p string) (Scheme, error) {
	data, err := files.ReadFile(p)
	if err != nil {
		return Scheme{}, err
	}
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Scheme{}, err
	}
	system := path.Base(path.Dir(p))
	stem := strings.TrimSuffix(strings.TrimSuffix(path.Base(p), ".yaml"), ".yml")
	s := Scheme{ID: system + "/" + stem, System: system, Variant: f.Variant, Colors: map[string]string{}}
	if system == "tinted8" {
		s.Name = f.Scheme.Family + " " + f.Scheme.Style
		for _, slot := range tinted8Slots {
			c, ok := f.Palette[slot[1]]
			if !ok {
				c, ok = tinted8Fallback(f.Palette, slot[1])
			}
			if !ok {
				return Scheme{}, fmt.Errorf("missing tinted8 color %s", slot[1])
			}
			s.Colors[slot[0]] = c
		}
		return s, nil
	}
	s.Name = f.Name
	s.Colors = f.Palette
	for i := range 16 {
		slot := fmt.Sprintf("base%02X", i)
		if _, ok := f.Palette[slot]; !ok {
			return Scheme{}, fmt.Errorf("missing %s", slot)
		}
	}
	return s, nil
}
