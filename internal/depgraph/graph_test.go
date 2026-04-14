package depgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_LinearChain(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a", Source: "gh:owner/a"})
	g.AddNode(&Package{Name: "b", Source: "gh:owner/b"})
	g.AddNode(&Package{Name: "c", Source: "gh:owner/c"})
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")

	result, err := g.Resolve("a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestResolve_Diamond(t *testing.T) {
	//   a
	//  / \
	// b   c
	//  \ /
	//   d
	g := NewGraph()
	g.AddNode(&Package{Name: "a", Source: "gh:owner/a"})
	g.AddNode(&Package{Name: "b", Source: "gh:owner/b"})
	g.AddNode(&Package{Name: "c", Source: "gh:owner/c"})
	g.AddNode(&Package{Name: "d", Source: "gh:owner/d"})
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")

	result, err := g.Resolve("a")
	require.NoError(t, err)
	assert.Contains(t, result, "a")
	assert.Contains(t, result, "b")
	assert.Contains(t, result, "c")
	assert.Contains(t, result, "d")
	assert.Len(t, result, 4)
}

func TestResolve_CycleDetection(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a", Source: "gh:owner/a"})
	g.AddNode(&Package{Name: "b", Source: "gh:owner/b"})
	g.AddNode(&Package{Name: "c", Source: "gh:owner/c"})
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "a")

	_, err := g.Resolve("a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
}

func TestResolve_SingleNode(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "solo", Source: "gh:owner/solo"})

	result, err := g.Resolve("solo")
	require.NoError(t, err)
	assert.Equal(t, []string{"solo"}, result)
}

func TestFilterInstalled(t *testing.T) {
	packages := []string{"a", "b", "c", "d"}
	installed := map[string]bool{"b": true, "d": true}

	result := FilterInstalled(packages, installed)
	assert.Equal(t, []string{"a", "c"}, result)
}

func TestFilterInstalled_NoneInstalled(t *testing.T) {
	packages := []string{"a", "b"}
	installed := map[string]bool{}

	result := FilterInstalled(packages, installed)
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestFilterInstalled_AllInstalled(t *testing.T) {
	packages := []string{"a", "b"}
	installed := map[string]bool{"a": true, "b": true}

	result := FilterInstalled(packages, installed)
	assert.Nil(t, result)
}

func TestReverseDependencies(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a"})
	g.AddNode(&Package{Name: "b"})
	g.AddNode(&Package{Name: "c"})
	g.AddEdge("a", "b")
	g.AddEdge("c", "b")

	reverse := g.ReverseDependencies()
	assert.ElementsMatch(t, []string{"a", "c"}, reverse["b"])
	assert.Empty(t, reverse["a"])
}

func TestReverseDependencies_NoDeps(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a"})

	reverse := g.ReverseDependencies()
	assert.Empty(t, reverse)
}

func TestReverseDependencies_MultipleDependents(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a"})
	g.AddNode(&Package{Name: "b"})
	g.AddNode(&Package{Name: "c"})
	g.AddNode(&Package{Name: "shared"})
	g.AddEdge("a", "shared")
	g.AddEdge("b", "shared")
	g.AddEdge("c", "shared")

	reverse := g.ReverseDependencies()
	assert.Len(t, reverse["shared"], 3)
}

func TestHasNode(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Package{Name: "a"})
	assert.True(t, g.HasNode("a"))
	assert.False(t, g.HasNode("b"))
}
