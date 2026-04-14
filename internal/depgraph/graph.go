package depgraph

import (
	"fmt"
	"strings"
)

// Package represents a resolved plugin package.
type Package struct {
	Name         string
	Source       string
	Dependencies []string
}

// Graph represents a dependency graph for plugin packages.
type Graph struct {
	nodes map[string]*Package
	edges map[string][]string
}

// NewGraph creates a new empty dependency graph.
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Package),
		edges: make(map[string][]string),
	}
}

// AddNode adds a package to the graph.
func (g *Graph) AddNode(pkg *Package) {
	g.nodes[pkg.Name] = pkg
}

// AddEdge declares a dependency from one package to another.
func (g *Graph) AddEdge(from, to string) {
	g.edges[from] = append(g.edges[from], to)
}

// HasNode checks if a package exists in the graph.
func (g *Graph) HasNode(name string) bool {
	_, ok := g.nodes[name]
	return ok
}

// GetNode returns a package by name.
func (g *Graph) GetNode(name string) *Package {
	return g.nodes[name]
}

// Resolve collects all packages in the graph using BFS, detecting cycles.
// Returns a flat list of all package names in the graph.
func (g *Graph) Resolve(root string) ([]string, error) {
	cycle := g.detectCycle(root)
	if cycle != nil {
		return nil, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " → "))
	}

	visited := make(map[string]bool)
	var result []string
	queue := []string{root}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true
		result = append(result, current)

		for _, dep := range g.edges[current] {
			if !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}

	return result, nil
}

// detectCycle uses DFS with a visiting marker to find cycles.
func (g *Graph) detectCycle(start string) []string {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	parent := make(map[string]string)

	var dfs func(node string) []string
	dfs = func(node string) []string {
		visiting[node] = true

		for _, dep := range g.edges[node] {
			if visiting[dep] {
				// Found a cycle — reconstruct the chain
				chain := []string{dep, node}
				current := node
				for current != dep {
					current = parent[current]
					if current == "" {
						break
					}
					chain = append(chain, current)
				}
				// Reverse the chain for readability
				for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
					chain[i], chain[j] = chain[j], chain[i]
				}
				return chain
			}
			if !visited[dep] {
				parent[dep] = node
				if cycle := dfs(dep); cycle != nil {
					return cycle
				}
			}
		}

		visiting[node] = false
		visited[node] = true
		return nil
	}

	return dfs(start)
}

// FilterInstalled removes already-installed packages from the list.
func FilterInstalled(packages []string, installed map[string]bool) []string {
	var toInstall []string
	for _, pkg := range packages {
		if !installed[pkg] {
			toInstall = append(toInstall, pkg)
		}
	}
	return toInstall
}

// ReverseDependencies returns a map of package → list of packages that depend on it.
func (g *Graph) ReverseDependencies() map[string][]string {
	reverse := make(map[string][]string)
	for from, deps := range g.edges {
		for _, to := range deps {
			reverse[to] = append(reverse[to], from)
		}
	}
	return reverse
}
