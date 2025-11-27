package random

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Shuffle performs a cryptographically secure shuffle of the slice.
func Shuffle[T any](slice []T) error {
	n := len(slice)
	for i := n - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("failed to generate random number: %w", err)
		}
		j := int(jBig.Int64())
		slice[i], slice[j] = slice[j], slice[i]
	}
	return nil
}

