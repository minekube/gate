package message

import (
	"errors"
	"fmt"
	"regexp"
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

var ValidIdentifierRegex = regexp.MustCompile(`[a-z0-9\\-_]*`)

// NewChannelIdentifier returns a new validated channel identifier.
func NewChannelIdentifier(namespace, name string) (ChannelIdentifier, error) {
	if len(namespace) == 0 {
		return nil, errors.New("namespace cannot be empty")
	}
	if len(name) == 0 {
		return nil, errors.New("name cannot be empty")
	}
	if !ValidIdentifierRegex.MatchString(namespace) {
		return nil, fmt.Errorf("namespace does not match regex %s", ValidIdentifierRegex.String())
	}
	if !ValidIdentifierRegex.MatchString(name) {
		return nil, fmt.Errorf("name does not match regex %s", ValidIdentifierRegex.String())
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

var _ ChannelIdentifier = (*channelIdentifier)(nil)

var (
	errIdentifierNoColon = errors.New("identifier does not contain a colon")
	errIdentifierEmpty   = errors.New("identifier is empty")
)

// ChannelIdentifierFrom creates a channel identifier from the specified Minecraft identifier string.
func ChannelIdentifierFrom(identifier string) (ChannelIdentifier, error) {
	colonPos := strings.Index(identifier, ":")
	if colonPos == -1 {
		return nil, errIdentifierNoColon
	}
	if colonPos+1 == len(identifier) {
		return nil, errIdentifierEmpty
	}
	namespace := identifier[:colonPos]
	name := identifier[colonPos+1:]
	return NewChannelIdentifier(namespace, name)
}
