package uuid

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"

	guuid "github.com/google/uuid"
)

type UUID guuid.UUID

// Empty UUID, all zeros
var Nil = UUID(guuid.Nil)

// String returns the string form of uuid,
// xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx , or "" if uuid is invalid.
func (i UUID) String() string {
	return guuid.UUID(i).String()
}

// Undashed returns the undashed string form of the uuid.
func (i UUID) Undashed() string {
	return hex.EncodeToString(i[:])
}

func (i UUID) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(i.String())), nil
}
func (i *UUID) UnmarshalJSON(b []byte) (err error) {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return fmt.Errorf("expected quoted uuid, but got %s: %w", b, err)
	}
	*i, err = Parse(s)
	return
}

// Parse decodes s into a UUID or returns an error.  Both the standard UUID
// forms of xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx and
// urn:uuid:xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx are decoded as well as the
// Microsoft encoding {xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx} and the raw hex
// encoding: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.
func Parse(s string) (UUID, error) {
	uuid, err := guuid.Parse(s)
	return UUID(uuid), err
}

// ParseBytes is like Parse, except it parses a byte slice instead of a string.
func ParseBytes(b []byte) (UUID, error) {
	uuid, err := guuid.ParseBytes(b)
	return UUID(uuid), err
}

// FromBytes creates a new UUID from a byte slice. Returns an error if the slice
// does not have a length of 16. The bytes are copied from the slice.
func FromBytes(b []byte) (UUID, error) {
	uuid, err := guuid.FromBytes(b)
	return UUID(uuid), err
}

func OfflinePlayerUUID(username string) UUID {
	const version = 3 // UUID v3
	uuid := md5.Sum([]byte("OfflinePlayer:" + username))
	uuid[6] = (uuid[6] & 0x0f) | uint8((version&0xf)<<4)
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // RFC 4122 variant
	return uuid
}

// New creates a new random UUID or panics.
func New() UUID { return UUID(guuid.New()) }
