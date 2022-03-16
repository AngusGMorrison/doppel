package doppel

// A CacheSchematic is an acyclic graph of TemplateSchematics.
type CacheSchematic map[string]*TemplateSchematic

// Clone returns a deep copy of the CacheSchematic.
func (cs CacheSchematic) Clone() CacheSchematic {
	dest := make(CacheSchematic, len(cs))
	for k, v := range cs {
		dest[k] = v.Clone()
	}
	return dest
}

// TemplateSchematic describes how to parse a template from a cached based
// template and zero or more template files.
//
// BaseTmplName may be an empty string, indicating a template without a base.
type TemplateSchematic struct {
	BaseTmplName string
	Filepaths    []string
}

// Clone returns a pointer to deep copy of the underlying TemplateSchematic.
func (ts *TemplateSchematic) Clone() *TemplateSchematic {
	dest := &TemplateSchematic{
		BaseTmplName: ts.BaseTmplName,
		Filepaths:    make([]string, len(ts.Filepaths)),
	}
	copy(dest.Filepaths, ts.Filepaths)
	return dest
}
