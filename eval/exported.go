// Copyright (c) 2026, the go-puppet/puppet authors
//
// SPDX-License-Identifier: BSD-3-Clause

package eval

import (
	"sync"

	"github.com/go-puppet/puppet/catalog"
)

// ExportedStore is the storeback seam for exported resources (`@@`). A host
// wires a real backend (e.g. PuppetDB via github.com/go-puppetdb/puppetdb) so
// that resources exported by one node's compilation can be collected by another
// node with `Type <<| query |>>`.
//
// StoreExported is called for every `@@` resource declared during compilation.
// CollectExported returns the exported resources of the given capitalized type
// (e.g. "Sshkey"), excluding those exported by excludeNode (a node never
// collects its own exports). Query filtering is applied by the evaluator, so an
// implementation may return a superset.
type ExportedStore interface {
	StoreExported(node string, r *catalog.Resource) error
	CollectExported(capType, excludeNode string) ([]*catalog.Resource, error)
}

// MemoryExportedStore is an in-memory [ExportedStore]. It is safe for concurrent
// use and is the default backend used in tests and single-process runs.
type MemoryExportedStore struct {
	mu     sync.Mutex
	byNode map[string][]*catalog.Resource
	byType map[string][]exportedEntry
}

type exportedEntry struct {
	node string
	res  *catalog.Resource
}

// NewMemoryExportedStore returns an empty in-memory exported-resource store.
func NewMemoryExportedStore() *MemoryExportedStore {
	return &MemoryExportedStore{
		byNode: map[string][]*catalog.Resource{},
		byType: map[string][]exportedEntry{},
	}
}

// StoreExported records r as exported by node.
func (m *MemoryExportedStore) StoreExported(node string, r *catalog.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byNode[node] = append(m.byNode[node], r)
	m.byType[r.Type] = append(m.byType[r.Type], exportedEntry{node: node, res: r})
	return nil
}

// CollectExported returns the exported resources of capType not exported by
// excludeNode.
func (m *MemoryExportedStore) CollectExported(capType, excludeNode string) ([]*catalog.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*catalog.Resource
	for _, entry := range m.byType[capType] {
		if entry.node == excludeNode {
			continue
		}
		out = append(out, entry.res)
	}
	return out, nil
}
