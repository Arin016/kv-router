package tokenizer

import (
	"strings"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

// Message represents a single chat message with a role and content.
type Message struct {
	Role    string
	Content string
}

// BlockHasher splits concatenated message content into fixed-size blocks
// and hashes each block with xxhash for fast prefix-deduplication.
type BlockHasher struct {
	BlockSize int
}

// HashPrefix concatenates all messages (role-prefixed) and returns a slice
// of xxhash digests, one per BlockSize-character block. The final partial
// block (if any) is also hashed.
func (bh *BlockHasher) HashPrefix(messages []Message) []uint64 {
	// Pre-calculate total length to avoid reallocation.
	totalLen := 0
	for i := range messages {
		totalLen += len(messages[i].Role) + 1 + len(messages[i].Content) // "role:content"
	}

	var b strings.Builder
	b.Grow(totalLen)

	for i := range messages {
		b.WriteString(messages[i].Role)
		b.WriteByte(':')
		b.WriteString(messages[i].Content)
	}

	str := b.String()
	n := len(str)
	if n == 0 {
		return nil
	}

	numBlocks := (n + bh.BlockSize - 1) / bh.BlockSize
	hashes := make([]uint64, 0, numBlocks)

	for offset := 0; offset < n; offset += bh.BlockSize {
		end := offset + bh.BlockSize
		if end > n {
			end = n
		}
		block := str[offset:end]
		// Zero-allocation hash: convert string to []byte without copy.
		hashes = append(hashes, xxhash.Sum64(unsafeBytes(block)))
	}

	return hashes
}

// PrefixMatch returns the length of the longest common prefix between
// two hash sequences. This is the number of leading blocks that are identical.
func PrefixMatch(a, b []uint64) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// unsafeBytes converts a string to a byte slice without allocation.
// The returned slice MUST NOT be modified.
func unsafeBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
