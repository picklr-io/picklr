package engine

import (
	"fmt"
	"strings"

	"github.com/picklr-io/picklr/internal/ir"
)

// DAG represents a directed acyclic graph of resources for dependency ordering.
type DAG struct {
	nodes    map[string]*dagNode
	order    []string // topological order (creation order)
	revOrder []string // reverse topological order (destruction order)
}

type dagNode struct {
	addr     string
	edges    []string // resources this node depends on
	revEdges []string // resources that depend on this node
}

// BuildDAG constructs a dependency graph from resources.
// It resolves both explicit DependsOn and implicit ptr:// references.
func BuildDAG(resources []*ir.Resource) (*DAG, error) {
	dag := &DAG{
		nodes: make(map[string]*dagNode),
	}

	// Build address map for lookups
	addrMap := make(map[string]*ir.Resource)
	for _, res := range resources {
		addr := resourceAddr(res)
		addrMap[addr] = res
		dag.nodes[addr] = &dagNode{addr: addr}
	}

	// Build edges from DependsOn and ptr:// references
	for _, res := range resources {
		addr := resourceAddr(res)
		node := dag.nodes[addr]

		// Explicit DependsOn
		for _, dep := range res.DependsOn {
			if _, ok := dag.nodes[dep]; ok {
				node.edges = append(node.edges, dep)
			}
		}

		// Implicit ptr:// references in properties
		refs := extractPtrRefs(res.Properties)
		for _, ref := range refs {
			depAddr := ptrRefToAddr(ref)
			if depAddr != "" {
				if _, ok := dag.nodes[depAddr]; ok {
					node.edges = append(node.edges, depAddr)
				}
			}
		}
	}

	// Build reverse edges
	for addr, node := range dag.nodes {
		for _, dep := range node.edges {
			dag.nodes[dep].revEdges = append(dag.nodes[dep].revEdges, addr)
		}
	}

	// Topological sort (Kahn's algorithm)
	order, err := dag.topoSort()
	if err != nil {
		return nil, err
	}
	dag.order = order

	// Reverse order for destruction
	dag.revOrder = make([]string, len(order))
	for i, addr := range order {
		dag.revOrder[len(order)-1-i] = addr
	}

	return dag, nil
}

// BuildDAGFromState constructs a dependency graph from state resources (for destroy).
func BuildDAGFromState(resources []*ir.ResourceState) (*DAG, error) {
	dag := &DAG{
		nodes: make(map[string]*dagNode),
	}

	for _, res := range resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		dag.nodes[addr] = &dagNode{addr: addr}

		// Build edges from Dependencies field
		for _, dep := range res.Dependencies {
			dag.nodes[addr] = &dagNode{addr: addr}
			dag.nodes[addr].edges = append(dag.nodes[addr].edges, dep)
		}
	}

	// Ensure all dependency nodes exist
	for _, node := range dag.nodes {
		for _, dep := range node.edges {
			if _, ok := dag.nodes[dep]; !ok {
				dag.nodes[dep] = &dagNode{addr: dep}
			}
		}
	}

	// Build reverse edges
	for addr, node := range dag.nodes {
		for _, dep := range node.edges {
			dag.nodes[dep].revEdges = append(dag.nodes[dep].revEdges, addr)
		}
	}

	order, err := dag.topoSort()
	if err != nil {
		return nil, err
	}
	dag.order = order

	dag.revOrder = make([]string, len(order))
	for i, addr := range order {
		dag.revOrder[len(order)-1-i] = addr
	}

	return dag, nil
}

// CreationOrder returns resources in dependency-respecting creation order.
func (d *DAG) CreationOrder() []string {
	return d.order
}

// DestructionOrder returns resources in reverse dependency order (safe for deletion).
func (d *DAG) DestructionOrder() []string {
	return d.revOrder
}

// topoSort performs Kahn's algorithm for topological sorting.
func (d *DAG) topoSort() ([]string, error) {
	inDegree := make(map[string]int)
	for addr := range d.nodes {
		inDegree[addr] = 0
	}
	for _, node := range d.nodes {
		for _, dep := range node.edges {
			inDegree[node.addr]++ // this node has an incoming edge from dep
			_ = dep
		}
	}

	// Recalculate: inDegree should count how many deps point TO this node
	for addr := range d.nodes {
		inDegree[addr] = len(d.nodes[addr].edges)
	}

	var queue []string
	for addr, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, addr)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		for _, dependent := range d.nodes[node].revEdges {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(d.nodes) {
		return nil, fmt.Errorf("dependency cycle detected in resource graph")
	}

	return sorted, nil
}

// ResourceAddrPublic returns the address of a resource (type.name). Exported for CLI use.
func ResourceAddrPublic(res *ir.Resource) string {
	return resourceAddr(res)
}

// Dependencies returns the list of dependencies for a given address.
func (d *DAG) Dependencies(addr string) []string {
	if node, ok := d.nodes[addr]; ok {
		return node.edges
	}
	return nil
}

// resourceAddr returns the address of a resource (type.name).
func resourceAddr(res *ir.Resource) string {
	t := res.Type
	if t == "" {
		t = "null_resource"
	}
	return fmt.Sprintf("%s.%s", t, res.Name)
}

// extractPtrRefs extracts all ptr:// references from a property value.
func extractPtrRefs(v any) []string {
	var refs []string
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "ptr://") {
			refs = append(refs, val)
		}
	case map[string]any:
		for _, v := range val {
			refs = append(refs, extractPtrRefs(v)...)
		}
	case map[any]any:
		for _, v := range val {
			refs = append(refs, extractPtrRefs(v)...)
		}
	case []any:
		for _, v := range val {
			refs = append(refs, extractPtrRefs(v)...)
		}
	}
	return refs
}

// ptrRefToAddr converts a ptr:// reference to a resource address.
// ptr://aws:EC2.Vpc/my-vpc/id -> aws:EC2.Vpc.my-vpc
func ptrRefToAddr(ref string) string {
	if !strings.HasPrefix(ref, "ptr://") {
		return ""
	}
	path := ref[6:] // Remove ptr://
	// Format: provider:Type/name/attribute
	// We need provider:Type.name as the address

	// Find the last slash (attribute) and second-to-last (name)
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	// parts[0] = "aws:EC2.Vpc", parts[1] = "my-vpc", parts[2] = "id" (optional)
	return fmt.Sprintf("%s.%s", parts[0], parts[1])
}
