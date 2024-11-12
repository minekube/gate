package hashutil

import (
	"crypto/sha1"
	"encoding/json"
)

// JsonHash returns the sha1 hash of the JSON representation of v.
func JsonHash(v any) ([]byte, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	h := sha1.Sum(j)
	return h[:], nil
}
