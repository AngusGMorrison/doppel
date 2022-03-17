// Package templatecache provides a concurrent, non-blocking cache of composable
// templates.
package templatecache

import (
	"errors"
	"fmt"
)

// Name represents the name of a template.
type Name string

// TemplateSchematic describes how to compile a template from from its parent
// and zero or more template-specific components.
type TemplateSchematic struct {
	Name, Parent   Name
	ComponentPaths []string
}

func (ts TemplateSchematic) Clone() TemplateSchematic {
	dest := TemplateSchematic{
		Parent:         ts.Parent,
		ComponentPaths: make([]string, len(ts.ComponentPaths)),
	}
	copy(dest.ComponentPaths, ts.ComponentPaths)

	return dest
}

type graph map[Name]TemplateSchematic

// CacheSchematic is an acylic graph of TemplateSchematics, describing the
// inheritance relationship between base templates and their children.
type CacheSchematic struct {
	graph
}

func NewCacheSchematic() CacheSchematic {
	return CacheSchematic{graph: make(graph)}
}

// Len returns the number of TemplateSchematics that make up the CacheSchematic.
func (cs CacheSchematic) Len() int {
	return len(cs.graph)
}

func (cs CacheSchematic) Clone() CacheSchematic {
	dest := CacheSchematic{graph: make(graph, len(cs.graph))}

	for k, v := range cs.graph {
		dest.graph[k] = v.Clone()
	}

	return dest
}

func (cs CacheSchematic) Add(ts TemplateSchematic) error {
	cs.graph[ts.Name] = ts

	if cyclic, err := cs.isCyclic(); cyclic {
		return fmt.Errorf("failed to add TemplateSchematic: %w", err)
	}

	return nil
}

// isCyclic reports whether a CacheSchematic contains a cycle. If true, the
// accompanying error describes which TemplateSchematics form part of the cycle.
func (cs CacheSchematic) isCyclic() (bool, error) {
	seen := make(map[Name]bool)

	// Keep track of TemplateSchematics seen in the current traversal with a
	// stack.
	var stack []Name

	// visit traverses all the nodes in the graph and returns an error if it
	// finds a cycle.
	var visit func(TemplateSchematic) error
	visit = func(currentSchematic TemplateSchematic) error {
		for _, previousSchematicName := range stack {
			if previousSchematicName == currentSchematic.Name {
				cycle := append(stack, currentSchematic.Name)

				return fmt.Errorf("cycle through %s (%v): %w",
					currentSchematic.Name, cycle, errCyclicGraph)
			}
		}

		stack = append(stack, currentSchematic.Name)

		var err error
		if !seen[currentSchematic.Name] {
			seen[currentSchematic.Name] = true

			if parentSchematic, ok := cs.graph[currentSchematic.Parent]; ok {
				err = visit(parentSchematic)
			}
		}

		stack = stack[:len(stack)-1]

		return err
	}

	for _, templateSchematic := range cs.graph {
		if err := visit(templateSchematic); err != nil {
			return true, err
		}
	}

	return false, nil
}

var errCyclicGraph = errors.New("graph is cyclic")
