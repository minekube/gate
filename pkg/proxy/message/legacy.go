package message

type legacyChannelIdentifier struct {
	name string
}

func NewLegacyChannelIdentifier(name string) ChannelIdentifier {
	return &legacyChannelIdentifier{name: name}
}

func (l *legacyChannelIdentifier) ID() string {
	return l.name
}

var _ ChannelIdentifier = (*legacyChannelIdentifier)(nil)
