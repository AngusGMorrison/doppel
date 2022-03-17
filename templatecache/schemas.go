package templatecache

import (
	"errors"
	"fmt"
)

// Name represents the name of a template.
type Name string

// TemplateSchema describes how to compile a template from from its parent
// and zero or more template-specific components.
type TemplateSchema struct {
	Name, Parent   Name
	ComponentPaths []string
}

func (ts TemplateSchema) Clone() TemplateSchema {
	dest := TemplateSchema{
		Parent:         ts.Parent,
		ComponentPaths: make([]string, len(ts.ComponentPaths)),
	}
	copy(dest.ComponentPaths, ts.ComponentPaths)

	return dest
}

type graph map[Name]TemplateSchema

// CacheSchema is an acylic graph of TemplateSchemas, describing the
// inheritance relationship between base templates and their children.
type CacheSchema struct {
	graph
}

func NewCacheSchema() CacheSchema {
	return CacheSchema{graph: make(graph)}
}

// Len returns the number of TemplateSchemas that make up the CacheSchema.
func (cs CacheSchema) Len() int {
	return len(cs.graph)
}

func (cs CacheSchema) Clone() CacheSchema {
	dest := CacheSchema{graph: make(graph, len(cs.graph))}

	for k, v := range cs.graph {
		dest.graph[k] = v.Clone()
	}

	return dest
}

func (cs CacheSchema) Add(ts TemplateSchema) error {
	cs.graph[ts.Name] = ts

	if cyclic, err := cs.isCyclic(); cyclic {
		return fmt.Errorf("failed to add TemplateSchema: %w", err)
	}

	return nil
}

// isCyclic reports whether a CacheSchema contains a cycle. If true, the
// accompanying error describes which TemplateSchemas form part of the cycle.
func (cs CacheSchema) isCyclic() (bool, error) {
	seen := make(map[Name]bool)

	// Keep track of TemplateSchemas seen in the current traversal with a
	// stack.
	var stack []Name

	// visit traverses all the nodes in the graph and returns an error if it
	// finds a cycle.
	var visit func(TemplateSchema) error
	visit = func(currentSchema TemplateSchema) error {
		for _, previousSchemaName := range stack {
			if previousSchemaName == currentSchema.Name {
				cycle := append(stack, currentSchema.Name)

				return fmt.Errorf("cycle through %s (%v): %w",
					currentSchema.Name, cycle, errCyclicGraph)
			}
		}

		stack = append(stack, currentSchema.Name)

		var err error
		if !seen[currentSchema.Name] {
			seen[currentSchema.Name] = true

			if parentSchema, ok := cs.graph[currentSchema.Parent]; ok {
				err = visit(parentSchema)
			}
		}

		stack = stack[:len(stack)-1]

		return err
	}

	for _, templateSchema := range cs.graph {
		if err := visit(templateSchema); err != nil {
			return true, err
		}
	}

	return false, nil
}

var errCyclicGraph = errors.New("graph is cyclic")
