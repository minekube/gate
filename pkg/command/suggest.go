package command

import "go.minekube.com/brigodier"

// SuggestFunc is a convenient function type implementing
// the brigodier.SuggestionProvider interface.
type SuggestFunc func(
	c *Context,
	b *brigodier.SuggestionsBuilder) *brigodier.Suggestions

var _ brigodier.SuggestionProvider = (*SuggestFunc)(nil)

func (s SuggestFunc) Suggestions(
	c *brigodier.CommandContext,
	b *brigodier.SuggestionsBuilder) *brigodier.Suggestions {
	return s(createContext(c), b)
}
