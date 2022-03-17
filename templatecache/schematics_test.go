package templatecache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	t.Run("adds_acyclic_template_schematics_to_graph", func(t *testing.T) {
		cs := NewCacheSchematic()
		base := TemplateSchematic{
			Name:           "base",
			Parent:         "",
			ComponentPaths: []string{"/path/to/template.gohtml"},
		}
		child := TemplateSchematic{
			Name:           "child",
			Parent:         "base",
			ComponentPaths: []string{"/path/to/component.gohtml"},
		}

		err := cs.Add(base)
		require.NoError(err)

		err = cs.Add(child)
		require.NoError(err)

		require.Equal(2, cs.Len())
	})

	t.Run("detects_single-node_cycles", func(t *testing.T) {
		cs := NewCacheSchematic()
		base := TemplateSchematic{
			Name:           "base",
			Parent:         "child",
			ComponentPaths: []string{"/path/to/template.gohtml"},
		}
		child := TemplateSchematic{
			Name:           "child",
			Parent:         "base",
			ComponentPaths: []string{"/path/to/component.gohtml"},
		}

		err := cs.Add(base)
		require.NoError(err)

		err = cs.Add(child)
		require.ErrorIs(err, errCyclicGraph)
	})

	t.Run("detects_multi-node_cycles", func(t *testing.T) {
		cs := NewCacheSchematic()
		base := TemplateSchematic{
			Name:           "base",
			Parent:         "grandchild",
			ComponentPaths: []string{"/path/to/template.gohtml"},
		}
		child := TemplateSchematic{
			Name:           "child",
			Parent:         "base",
			ComponentPaths: []string{"/path/to/component.gohtml"},
		}
		grandchild := TemplateSchematic{
			Name:           "grandchild",
			Parent:         "child",
			ComponentPaths: []string{"/path/to/component.gohtml"},
		}

		err := cs.Add(base)
		require.NoError(err)

		err = cs.Add(child)
		require.NoError(err)

		err = cs.Add(grandchild)
		require.ErrorIs(err, errCyclicGraph)
	})
}
