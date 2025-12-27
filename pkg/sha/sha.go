package sha

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// QuickXorHash implements Microsoft's QuickXorHash algorithm
// Used by OneDrive to generate file hashes
type QuickXorHash struct {
	data        [20]byte // 160-bit hash (20 bytes)
	lengthSoFar uint64
	shiftSoFar  int
}

const (
	widthInBits = 160
	shift       = 11
)

// NewQuickXorHash creates a new QuickXorHash instance
func NewQuickXorHash() *QuickXorHash {
	return &QuickXorHash{}
}

// Write implements io.Writer interface
func (q *QuickXorHash) Write(p []byte) (n int, err error) {
	currentShift := q.shiftSoFar

	// XOR each byte into the hash
	for _, b := range p {
		// Position of this byte in the 160-bit vector
		vectorArrayIndex := currentShift / 8
		vectorOffset := currentShift % 8

		// Circular shift the hash
		q.data[vectorArrayIndex] ^= b << vectorOffset

		// Handle wraparound for the circular shift
		if vectorOffset > 0 && vectorArrayIndex < 19 {
			q.data[vectorArrayIndex+1] ^= b >> (8 - vectorOffset)
		} else if vectorOffset > 0 {
			// Wrap around to the beginning
			q.data[0] ^= b >> (8 - vectorOffset)
		}

		// Update shift position
		currentShift = (currentShift + shift) % widthInBits
	}

	q.shiftSoFar = currentShift
	q.lengthSoFar += uint64(len(p))

	return len(p), nil
}

// Sum returns the QuickXorHash as a base64-encoded string
func (q *QuickXorHash) Sum() string {
	// XOR the file length into the hash
	lengthBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		lengthBytes[i] = byte(q.lengthSoFar >> (8 * i))
	}

	// XOR length into the last 8 bytes
	for i := 0; i < 8; i++ {
		q.data[19-i] ^= lengthBytes[i]
	}

	// Return base64-encoded hash
	return base64.StdEncoding.EncodeToString(q.data[:])
}

// ComputeQuickXorHash calculates the QuickXorHash for a file
func ComputeQuickXorHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := NewQuickXorHash()

	// Read file in chunks and update hash
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
	}

	return hash.Sum(), nil
}