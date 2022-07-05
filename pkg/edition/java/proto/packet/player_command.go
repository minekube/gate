package packet

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	maxNumArguments    = 8
	maxLengthArguments = 16
)

var errLimitsViolation = errs.NewSilentErr("command arguments incorrect size")

type PlayerCommand struct {
	Unsigned      bool
	Command       string
	Timestamp     time.Time
	Salt          int64
	SignedPreview bool // Good god. Please no.
	Arguments     map[string][]byte
}

// NewPlayerCommand returns a new PlayerCommand packet based on a command and list of arguments.
func NewPlayerCommand(command string, arguments []string, timestamp time.Time) *PlayerCommand {
	args := make(map[string][]byte, len(arguments))
	for _, arg := range arguments {
		args[arg] = []byte{}
	}
	return &PlayerCommand{
		Unsigned:      true,
		Command:       command,
		Timestamp:     timestamp,
		Salt:          0,
		SignedPreview: false,
		Arguments:     args,
	}
}

func (p *PlayerCommand) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, p.Command)
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, p.Timestamp.UnixMilli())
	if err != nil {
		return err
	}
	if p.Unsigned {
		err = util.WriteInt64(wr, 0)
	} else {
		err = util.WriteInt64(wr, p.Salt)
	}
	if err != nil {
		return err
	}

	if len(p.Arguments) > maxNumArguments {
		return fmt.Errorf("%w: max is %d but was %d", errLimitsViolation, maxNumArguments, len(p.Arguments))
	}
	err = util.WriteVarInt(wr, len(p.Arguments))
	if err != nil {
		return err
	}
	for a, b := range p.Arguments {
		// What annoys me is that this isn't "sorted"
		err = util.WriteString(wr, a)
		if err != nil {
			return err
		}
		if p.Unsigned {
			err = util.WriteBytes(wr, []byte{})
		} else {
			err = util.WriteBytes(wr, b)
		}
		if err != nil {
			return err
		}
	}
	return util.WriteBool(wr, p.SignedPreview)
}

func (p *PlayerCommand) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.Command, err = util.ReadStringMax(rd, MaxServerBoundMessageLength)
	if err != nil {
		return err
	}
	timestamp, err := util.ReadInt64(rd)
	if err != nil {
		return err
	}
	p.Timestamp = time.UnixMilli(timestamp)

	p.Salt, err = util.ReadInt64(rd)
	if err != nil {
		return err
	}
	if p.Salt == 0 {
		p.Unsigned = true
	}

	mapSize, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	if mapSize > maxNumArguments {
		return fmt.Errorf("%w: max is %d but was %d", errLimitsViolation, maxNumArguments, mapSize)
	}
	// Mapped as Argument : signature
	entries := make(map[string][]byte, mapSize)
	for i := 0; i < mapSize; i++ {
		a, err := util.ReadStringMax(rd, maxLengthArguments)
		if err != nil {
			return err
		}
		var b []byte
		readBytes := util.DefaultMaxStringSize
		if p.Unsigned {
			readBytes = 0
		}
		b, err = util.ReadBytesLen(rd, readBytes)
		if err != nil {
			return err
		}
		entries[a] = b
	}
	p.Arguments = entries

	p.SignedPreview, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if p.Unsigned && p.SignedPreview {
		return errPreviewSignatureMissing
	}
	return nil
}

func (p *PlayerCommand) SignedContainer(signer crypto.IdentifiedKey, sender uuid.UUID, mustSign bool) (*crypto.SignedChatCommand, error) {
	if p.Unsigned {
		if mustSign {
			return nil, errInvalidSignature
		}
		return nil, nil
	}
	salt := new(bytes.Buffer)
	_ = util.WriteInt64(salt, p.Salt)
	return &crypto.SignedChatCommand{
		Command:       p.Command,
		Signer:        signer.SignedPublicKey(),
		Expiry:        p.Timestamp,
		Salt:          salt.Bytes(),
		Sender:        sender,
		SignedPreview: p.SignedPreview,
		Signatures:    p.Arguments,
	}, nil
}

var _ proto.Packet = (*PlayerCommand)(nil)
