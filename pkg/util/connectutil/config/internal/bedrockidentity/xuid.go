package bedrockidentity

import (
	"crypto/sha1" //nolint:gosec // Required for Gate/Floodgate's established UUID namespace contract.
	"errors"
	"strconv"

	"go.minekube.com/gate/pkg/util/uuid"
)

const floodgateXUIDNamespace = "FloodgateXUID:"

func CanonicalXUIDFromInt64(value int64) (string, error) {
	if value <= 0 {
		return "", errors.New("xuid must be a positive signed 64-bit integer")
	}
	return strconv.FormatInt(value, 10), nil
}

func ParseCanonicalXUID(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
		return 0, errors.New("xuid must be canonical positive decimal within the signed 64-bit range")
	}
	return parsed, nil
}

func DerivedUUIDForXUID(value string) (string, error) {
	if _, err := ParseCanonicalXUID(value); err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(floodgateXUIDNamespace + value)) //nolint:gosec // Compatibility UUID, not a cryptographic signature.
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	id, err := uuid.FromBytes(sum[:16])
	if err != nil || id == uuid.Nil {
		return "", errors.New("could not derive namespaced xuid uuid")
	}
	return id.String(), nil
}
