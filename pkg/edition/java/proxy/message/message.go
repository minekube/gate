package message

import (
	"errors"
	"fmt"
	"regexp"
)

// ChannelIdentifier is a channel identifier for use with plugin messaging.
type ChannelIdentifier interface {
	// Returns the channel identifier.
	ID() string
}

// ChannelMessageSink can receive plugin messages.
type ChannelMessageSink interface {
	// Sends a plugin message to the channel with id.
	SendPluginMessage(id ChannelIdentifier, data []byte) error
}

// ChannelMessageSource is a source of plugin messages.
type ChannelMessageSource interface{}

// ChannelRegistrar is an interface to register and
// unregister ChannelIdentifiers for the proxy to listen on.
type ChannelRegistrar interface {
	// Registers the specified message identifiers to listen on so you can
	// intercept plugin messages on the channel using the PluginMessageEvent.
	Register(ids ...ChannelIdentifier)
	// Removes the intent to listen for the specified channels.
	Unregister(ids ...ChannelIdentifier)
}

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
