/*

UUID is a bad name. What I want is unique identifiers with enough entropy to avoid
collisions.

*/
package uuid

import (
	"crypto/rand"
	"encoding/hex"
)

// Returns a random hex-string of 16 bytes
func UniqueId() (string, error) {
	uuid := make([]byte, 16)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}

	return hex.EncodeToString(uuid), nil
}
