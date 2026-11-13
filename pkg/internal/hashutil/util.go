package hashutil

import (
	"crypto/sha1"
	"encoding/json"
)

// JsonHash returns the sha1 hash of the JSON representation of v.
//
// SHA-1 is intentional and has no security consequence here: this is only an
// in-process "did this config object change" fingerprint, compared against the
// previous value in the same process (pkg/gate/api.go, pkg/gate/connect.go).
// It is never persisted, transmitted, or part of an integrity/authentication
// decision, and a collision would at worst skip a service restart - the config
// content itself is what gets applied. Swapping in SHA-256 is optional hygiene
// at most, and any swap must cover all callers at once.
// Accepted scanner finding: see docs/accepted-crypto-primitives.md.
func JsonHash(v any) ([]byte, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	h := sha1.Sum(j)
	return h[:], nil
}
