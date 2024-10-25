package util

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Tnze/go-mc/nbt"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type (
	// BinaryTag is a binary tag.
	BinaryTag = nbt.RawMessage
	// CompoundBinaryTag is a compound binary tag.
	CompoundBinaryTag = BinaryTag
)

// ReadBinaryTag reads a binary tag from the provided reader.
func ReadBinaryTag(r io.Reader, protocol proto.Protocol) (bt BinaryTag, err error) {
	// Read the type
	bt.Type, err = ReadByte(r)
	if err != nil {
		return bt, fmt.Errorf("error reading binary tag type: %w", err)
	}

	// Skip bytes if protocol version is less than 1.20.2.
	if protocol.Lower(version.Minecraft_1_20_2) {
		_, err = ReadUint16(r)
		if err != nil {
			return bt, fmt.Errorf("error skipping bytes: %w", err)
		}
	}

	// use io.MultiReader() to reassemble the reader to use for decoding
	// the binary tag without the skipped bytes
	mr := io.MultiReader(bytes.NewReader([]byte{bt.Type}), r)

	// Read the data
	dec := nbt.NewDecoder(mr)
	dec.NetworkFormat(true) // skip tag name

	if _, err = dec.Decode(&bt); err != nil {
		return bt, fmt.Errorf("error decoding binary tag: %w", err)
	}

	return bt, nil
}

// ReadCompoundTag reads a compound binary tag from the provided reader.
func ReadCompoundTag(r io.Reader, protocol proto.Protocol) (CompoundBinaryTag, error) {
	bt, err := ReadBinaryTag(r, protocol)
	if err != nil {
		return bt, err
	}
	if bt.Type != nbt.TagCompound {
		return bt, fmt.Errorf("expected root tag to be a compound tag, got %v", bt.Type)
	}
	return bt, nil
}

// WriteBinaryTag writes a binary tag to the provided writer.
func WriteBinaryTag(w io.Writer, protocol proto.Protocol, bt BinaryTag) error {
	// Write the type
	if err := WriteByte(w, bt.Type); err != nil {
		return fmt.Errorf("error writing binary tag type: %w", err)
	}
	if protocol.Lower(version.Minecraft_1_20_2) {
		// Empty name
		if err := WriteUint16(w, 0); err != nil {
			return fmt.Errorf("error writing binary tag name: %w", err)
		}
	}
	// Write the data
	if _, err := w.Write(bt.Data); err != nil {
		return fmt.Errorf("error writing binary tag data: %w", err)
	}
	return nil
}
