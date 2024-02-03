package velocity

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"go.minekube.com/gate/pkg/edition/java/profile"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	IpForwardingChannel          = "velocity:player_info"
	DefaultForwardingVersion     = 1
	WithKeyForwardingVersion     = 2
	WithKeyV2ForwardingVersion   = 3
	LazySessionForwardingVersion = 4
	ForwardingMaxVersion         = LazySessionForwardingVersion
)

// ConnectedPlayer represents a connected player.
type ConnectedPlayer interface {
	ID() uuid.UUID
	Username() string
	GameProfile() profile.GameProfile
	Protocol() proto.Protocol
	IdentifiedKey() crypto.IdentifiedKey
}

// CreateForwardingData creates the forwarding data for the given player in the Velocity format.
func CreateForwardingData(
	hmacSecret []byte, address string,
	player ConnectedPlayer, requestedVersion int,
) ([]byte, error) {
	forwarded := bytes.NewBuffer(make([]byte, 0, 2048))

	actualVersion := findForwardingVersion(requestedVersion, player)

	err := protoutil.WriteVarInt(forwarded, actualVersion)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, address)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteUUID(forwarded, player.ID())
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, player.Username())
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteProperties(forwarded, player.GameProfile().Properties)
	if err != nil {
		return nil, err
	}

	// This serves as additional redundancy. The key normally is stored in the
	// login start to the server, but some setups require this.
	if actualVersion >= WithKeyForwardingVersion &&
		actualVersion < LazySessionForwardingVersion {
		playerKey := player.IdentifiedKey()
		if playerKey == nil {
			return nil, errors.New("player auth key missing")
		}
		err = crypto.WritePlayerKey(forwarded, playerKey)
		if err != nil {
			return nil, err
		}

		// Provide the signer UUID since the UUID may differ from the
		// assigned UUID. Doing that breaks the signatures anyway but the server
		// should be able to verify the key independently.
		if actualVersion >= WithKeyV2ForwardingVersion {
			if playerKey.SignatureHolder() != uuid.Nil {
				_ = protoutil.WriteBool(forwarded, true)
				_ = protoutil.WriteUUID(forwarded, playerKey.SignatureHolder())
			} else {
				// Should only not be provided if the player was connected
				// as offline-mode and the signer UUID was not backfilled
				_ = protoutil.WriteBool(forwarded, false)
			}
		}
	}

	mac := hmac.New(sha256.New, hmacSecret)
	_, err = mac.Write(forwarded.Bytes())
	if err != nil {
		return nil, err
	}

	// final
	data := bytes.NewBuffer(make([]byte, 0, mac.Size()+forwarded.Len()))
	_, err = data.Write(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	_, err = data.Write(forwarded.Bytes())
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

// find velocity forwarding version
func findForwardingVersion(requested int, player ConnectedPlayer) int {
	// Ensure we are in range
	requested = min(requested, ForwardingMaxVersion)
	if requested > DefaultForwardingVersion {
		if player.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
			if requested >= LazySessionForwardingVersion {
				return LazySessionForwardingVersion
			}
			return DefaultForwardingVersion
		}
		if key := player.IdentifiedKey(); key != nil {
			if revision := key.KeyRevision(); revision != nil {
				switch revision {
				case keyrevision.GenericV1:
					return WithKeyForwardingVersion
				// Since V2 is not backwards compatible we have to throw the key if v2 and requested is v1
				case keyrevision.LinkedV2:
					if requested >= WithKeyV2ForwardingVersion {
						return WithKeyV2ForwardingVersion
					}
					return DefaultForwardingVersion
				}
			}
		}
	}
	return DefaultForwardingVersion
}
