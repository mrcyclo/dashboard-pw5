package widget

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
	"time"

	"claudedash/internal/media"
	"claudedash/internal/usage"
)

// Data is everything a widget may draw from.
type Data struct {
	Now   time.Time
	Usage usage.Snapshot
	Media media.State
	// Options holds each kind's chosen option values; see Configurable.
	Options map[string]map[string]string
	// Lighten holds how strongly each kind's panel lightens the background.
	Lighten map[string]float64
	// Apps lists the programs of the apps widget, in order.
	Apps []AppTile
}

// Option returns the value chosen for a kind's option, or fallback when none was.
func (d Data) Option(kind, key, fallback string) string {
	if value, ok := d.Options[kind][key]; ok && value != "" {
		return value
	}
	return fallback
}

// Choice is one value an option can take, with the words the window shows for it.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Option is a setting a widget offers besides its size.
type Option struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Choices []Choice `json:"choices"`
	Default string   `json:"default"`
}

// Configurable is a Kind with options. The window lists them under the widget.
type Configurable interface {
	Options() []Option
}

// Kind is one type of widget. Adding a widget means writing a Kind and listing it in Default.
type Kind interface {
	// Name is the stable id stored in the config and sent to the Kindle.
	Name() string
	Label() string
	// Icon names the lucide icon the window shows next to the widget.
	Icon() string
	// Sizes the widget can be drawn at, as "<columns>x<rows>", smallest first.
	Sizes() []string
	Build(size string, d Data) Node
}

// ActionFunc runs a button's action on the PC.
type ActionFunc func() error

// Actor is a Kind whose buttons trigger actions, named "<kind>.<action>".
type Actor interface {
	Actions() map[string]ActionFunc
}

type Catalog struct {
	kinds   []Kind
	actions map[string]ActionFunc
}

func NewCatalog(kinds ...Kind) *Catalog {
	c := &Catalog{kinds: kinds, actions: map[string]ActionFunc{}}
	for _, k := range kinds {
		actor, ok := k.(Actor)
		if !ok {
			continue
		}
		for name, fn := range actor.Actions() {
			c.actions[name] = fn
		}
	}
	return c
}

// Default lists every widget the dashboard offers, in the order the window shows them.
func Default(do func(cmd string) (media.State, error)) *Catalog {
	return NewCatalog(clockKind{}, claudeKind{}, NewMedia(do), appsKind{})
}

func (c *Catalog) All() []Kind {
	return c.kinds
}

func (c *Catalog) Get(name string) (Kind, bool) {
	for _, k := range c.kinds {
		if k.Name() == name {
			return k, true
		}
	}
	return nil, false
}

func (c *Catalog) Action(name string) (ActionFunc, bool) {
	fn, ok := c.actions[name]
	return fn, ok
}

// Supports reports whether kind can be drawn at size.
func (c *Catalog) Supports(kind, size string) bool {
	k, ok := c.Get(kind)
	if !ok {
		return false
	}
	for _, s := range k.Sizes() {
		if s == size {
			return true
		}
	}
	return false
}

//go:embed icons/*.svg
var icons embed.FS

// IconSVG returns an icon a tree may name. The Kindle downloads and keeps them.
func IconSVG(name string) ([]byte, bool) {
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return nil, false
	}
	data, err := icons.ReadFile("icons/" + name + ".svg")
	if err != nil {
		return nil, false
	}
	return data, true
}

func IconHash(name string) string {
	data, ok := IconSVG(name)
	if !ok {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// OptionsOf lists a kind's options, or none.
func (c *Catalog) OptionsOf(kind string) []Option {
	k, ok := c.Get(kind)
	if !ok {
		return nil
	}
	configurable, ok := k.(Configurable)
	if !ok {
		return nil
	}
	return configurable.Options()
}

// ValidOption reports whether value is one of the choices of kind's option key.
func (c *Catalog) ValidOption(kind, key, value string) bool {
	for _, o := range c.OptionsOf(kind) {
		if o.Key != key {
			continue
		}
		for _, choice := range o.Choices {
			if choice.Value == value {
				return true
			}
		}
	}
	return false
}
