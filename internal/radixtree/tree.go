package radixtree

import (
	"sync"
	"sync/atomic"
	"time"
)

// CacheEntry records that a specific backend has cached content at this prefix.
type CacheEntry struct {
	BackendID  string
	InsertedAt time.Time
	LastAccess time.Time
	HitCount   int64
}

// Node represents a single node in the radix tree keyed by uint64 block hashes.
type Node struct {
	children map[uint64]*Node
	Entries  []CacheEntry
}

func newNode() *Node {
	return &Node{
		children: make(map[uint64]*Node),
	}
}

// TreeStats exposes metrics about the tree's current state.
type TreeStats struct {
	NodeCount  int64
	EntryCount int64
	MaxDepth   int
}

// Tree is a concurrent radix tree mapping sequences of uint64 block hashes
// to backend cache entries. Optimized for read-heavy workloads via sync.RWMutex.
type Tree struct {
	root *Node
	mu   sync.RWMutex

	// Atomic counters for O(1) stats without tree walks.
	nodeCount  atomic.Int64
	entryCount atomic.Int64
}

// New creates an empty radix tree.
func New() *Tree {
	t := &Tree{
		root: newNode(),
	}
	t.nodeCount.Store(1) // root node
	return t
}

// Insert records that the given block-hash prefix was sent to backendID.
// If backendID already has an entry at this exact prefix, the existing entry
// is updated (hit count incremented, LastAccess refreshed).
func (t *Tree) Insert(hashes []uint64, backendID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for _, h := range hashes {
		child, exists := node.children[h]
		if !exists {
			child = newNode()
			node.children[h] = child
			t.nodeCount.Add(1)
		}
		node = child
	}

	// Check if backend already exists at this node.
	now := time.Now()
	for i := range node.Entries {
		if node.Entries[i].BackendID == backendID {
			node.Entries[i].LastAccess = now
			node.Entries[i].HitCount++
			return
		}
	}

	node.Entries = append(node.Entries, CacheEntry{
		BackendID:  backendID,
		InsertedAt: now,
		LastAccess: now,
		HitCount:   1,
	})
	t.entryCount.Add(1)
}

// Lookup returns a map of {backendID: matched_blocks_count} for all backends
// that have any prefix of the given hash sequence cached. Traverses the tree
// along the hash path, accumulating entries found at each depth.
func (t *Tree) Lookup(hashes []uint64) map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]int)
	node := t.root

	// Collect entries at root (zero-length prefix matches everything).
	for _, e := range node.Entries {
		result[e.BackendID] = 0
	}

	for depth, h := range hashes {
		child, exists := node.children[h]
		if !exists {
			break
		}
		node = child
		// depth+1 because we've traversed depth+1 edges (blocks matched).
		for _, e := range node.Entries {
			// Keep the longest prefix match per backend.
			if current, ok := result[e.BackendID]; !ok || depth+1 > current {
				result[e.BackendID] = depth + 1
			}
		}
	}

	return result
}

// Touch updates LastAccess for a specific backend at the given prefix.
// No-op if the backend has no entry at that exact prefix.
func (t *Tree) Touch(hashes []uint64, backendID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for _, h := range hashes {
		child, exists := node.children[h]
		if !exists {
			return
		}
		node = child
	}

	now := time.Now()
	for i := range node.Entries {
		if node.Entries[i].BackendID == backendID {
			node.Entries[i].LastAccess = now
			node.Entries[i].HitCount++
			return
		}
	}
}

// Evict removes the N oldest entries (by LastAccess) for a given backend,
// implementing LRU eviction. Walks the entire tree to find all entries
// belonging to the backend, sorts by LastAccess ascending, and removes
// up to count entries.
func (t *Tree) Evict(backendID string, count int) {
	if count <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Collect all entries for this backend with their node references.
	type entryRef struct {
		node  *Node
		index int
		entry CacheEntry
	}

	var refs []entryRef
	var walk func(n *Node)
	walk = func(n *Node) {
		for i, e := range n.Entries {
			if e.BackendID == backendID {
				refs = append(refs, entryRef{node: n, index: i, entry: e})
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(t.root)

	if len(refs) == 0 {
		return
	}

	// Sort by LastAccess ascending (oldest first) using insertion sort
	// since eviction counts are typically small.
	for i := 1; i < len(refs); i++ {
		for j := i; j > 0 && refs[j].entry.LastAccess.Before(refs[j-1].entry.LastAccess); j-- {
			refs[j], refs[j-1] = refs[j-1], refs[j]
		}
	}

	// Evict up to count oldest entries.
	toEvict := count
	if toEvict > len(refs) {
		toEvict = len(refs)
	}

	// Remove entries from their nodes. Process in reverse index order per node
	// to avoid index invalidation.
	type nodeRemoval struct {
		node    *Node
		indices []int
	}
	removals := make(map[*Node][]int)
	for i := 0; i < toEvict; i++ {
		ref := refs[i]
		removals[ref.node] = append(removals[ref.node], ref.index)
	}

	for node, indices := range removals {
		// Sort indices descending so removal doesn't shift later indices.
		for i := 1; i < len(indices); i++ {
			for j := i; j > 0 && indices[j] > indices[j-1]; j-- {
				indices[j], indices[j-1] = indices[j-1], indices[j]
			}
		}
		for _, idx := range indices {
			node.Entries = append(node.Entries[:idx], node.Entries[idx+1:]...)
		}
	}

	t.entryCount.Add(-int64(toEvict))
}

// Stats returns current tree metrics. Node and entry counts are maintained
// atomically, but max depth requires a tree walk under read lock.
func (t *Tree) Stats() TreeStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	maxDepth := 0
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		for _, child := range n.children {
			walk(child, depth+1)
		}
	}
	walk(t.root, 0)

	return TreeStats{
		NodeCount:  t.nodeCount.Load(),
		EntryCount: t.entryCount.Load(),
		MaxDepth:   maxDepth,
	}
}
