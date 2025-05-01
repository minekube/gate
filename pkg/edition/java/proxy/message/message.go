package message

import (
	"errors"
	"fmt"
	"strings"
)

// ChannelIdentifier is a channel identifier for use with plugin messaging.
type ChannelIdentifier interface {
	// ID returns the channel identifier.
	ID() string
}

// ChannelMessageSink can receive plugin messages.
type ChannelMessageSink interface {
	// SendPluginMessage sends a plugin message to the channel with id.
	SendPluginMessage(id ChannelIdentifier, data []byte) error
}

// ChannelMessageSource is a source of plugin messages.
type ChannelMessageSource any

const DefaultNamespace = "minecraft"

// channelIdentifier is a Minecraft 1.13+ plugin channel identifier.
type channelIdentifier struct {
	namespace, name string
}

var (
	ErrNamespaceEmpty   = errors.New("namespace cannot be empty")
	ErrNameEmpty        = errors.New("name cannot be empty")
	ErrNamespaceInvalid = errors.New("invalid namespace")
	ErrNameInvalid      = errors.New("invalid name")
)

// NewChannelIdentifier returns a new validated channel identifier.
func NewChannelIdentifier(namespace, name string) (ChannelIdentifier, error) {
	if len(namespace) == 0 {
		return nil, ErrNamespaceEmpty
	}
	if len(name) == 0 {
		return nil, ErrNameEmpty
	}
	for _, char := range namespace {
		if !allowedInNamespace(char) {
			return nil, fmt.Errorf("%w: namespace %s has invalid character %c", ErrNamespaceInvalid, namespace, char)
		}
	}
	for _, char := range name {
		if !allowedInValue(char) {
			return nil, fmt.Errorf("%w: name %s has invalid character %c", ErrNameInvalid, name, char)
		}
	}
	return &channelIdentifier{
		namespace: namespace,
		name:      name,
	}, nil
}

func NewDefaultNamespace(name string) (ChannelIdentifier, error) {
	return NewChannelIdentifier(DefaultNamespace, name)
}

func (m *channelIdentifier) Namespace() string {
	return m.namespace
}

func (m *channelIdentifier) Name() string {
	return m.name
}

func (m *channelIdentifier) ID() string {
	return fmt.Sprintf("%s:%s", m.namespace, m.name)
}

func (m *channelIdentifier) String() string {
	return m.ID()
}

var _ ChannelIdentifier = (*channelIdentifier)(nil)

// ChannelIdentifierFrom creates a channel identifier from the specified Minecraft identifier string.
func ChannelIdentifierFrom(identifier string) (ChannelIdentifier, error) {
	colonPos := strings.Index(identifier, ":")
	if colonPos == -1 {
		return NewChannelIdentifier(DefaultNamespace, identifier)
	} else if colonPos == 0 {
		return NewChannelIdentifier(DefaultNamespace, identifier[1:])
	}
	namespace := identifier[:colonPos]
	name := identifier[colonPos+1:]
	return NewChannelIdentifier(namespace, name)
}

// allowedInNamespace checks if a character is allowed in a namespace identifier.
// Valid characters are lowercase a-z, 0-9, underscore, hyphen, and period.
func allowedInNamespace(character rune) bool {
	return character == '_' || character == '-' ||
		(character >= 'a' && character <= 'z') ||
		(character >= '0' && character <= '9') ||
		character == '.'
}

// allowedInValue checks if a character is allowed in a value identifier.
// Valid characters are lowercase a-z, 0-9, underscore, hyphen, period, and forward slash.
func allowedInValue(character rune) bool {
	return character == '_' || character == '-' ||
		(character >= 'a' && character <= 'z') ||
		(character >= '0' && character <= '9') ||
		character == '.' || character == '/'
}
