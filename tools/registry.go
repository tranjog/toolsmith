package tools

type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

type Registry struct {
	byName map[string]ToolDefinition
	order  []string
}

func NewRegistry(defs ...ToolDefinition) *Registry {
	r := &Registry{byName: make(map[string]ToolDefinition, len(defs)), order: make([]string, 0, len(defs))}
	for _, d := range defs {
		if _, exists := r.byName[d.Name]; exists {
			continue
		}
		r.byName[d.Name] = d
		r.order = append(r.order, d.Name)
	}
	return r
}

func (r *Registry) Get(name string) (ToolDefinition, bool) {
	d, ok := r.byName[name]
	return d, ok
}

func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

func (r *Registry) All() []ToolDefinition {
	out := make([]ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}

func (r *Registry) Catalog() []CatalogEntry {
	out := make([]CatalogEntry, 0, len(r.order))
	for _, name := range r.order {
		d := r.byName[name]
		desc := d.ShortDescription
		if desc == "" {
			desc = d.Description
		}
		out = append(out, CatalogEntry{Name: d.Name, Description: desc, Category: d.Category})
	}
	return out
}
