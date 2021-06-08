package command

import (
	"context"
	"errors"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/permission"
	"strings"
)

// Manager is a command manager for
// registering and executing proxy commands.
type Manager struct{ brigodier.Dispatcher }

// Source is the invoker of a command.
// It could be a player or the console/terminal.
type Source interface {
	permission.Subject
	// SendMessage sends a message component to the invoker.
	SendMessage(msg component.Component) error
}

// SourceFromContext retrieves the Source from a command's context.
func SourceFromContext(ctx context.Context) Source {
	s := ctx.Value(sourceCtxKey)
	if s == nil {
		return nil
	}
	src, _ := s.(Source)
	return src
}

// Context wraps the context for a brigodier.Command.
type Context struct {
	*brigodier.CommandContext
	Source
}

// RequiresContext wraps the context for a brigodier.RequireFn.
type RequiresContext struct {
	context.Context
	Source
}

// Command wraps the context for a brigodier.Command.
func Command(fn func(c *Context) error) brigodier.Command {
	return brigodier.CommandFunc(func(c *brigodier.CommandContext) error {
		return fn(&Context{
			CommandContext: c,
			Source:         SourceFromContext(c),
		})
	})
}

// Requires wraps the context for a brigodier.RequireFn.
func Requires(fn func(c *RequiresContext) bool) func(context.Context) bool {
	return func(ctx context.Context) bool {
		return fn(&RequiresContext{
			Context: ctx,
			Source:  SourceFromContext(ctx),
		})
	}
}

// ParseResults are the parse results of a parsed command input.
//
// It overlays brigodier.ParseResults to make clear that Manager.Execute
// must only get parse results returned by Manager.Parse.
type ParseResults brigodier.ParseResults

// Parse stores a required command invoker Source in ctx,
// parses the command and returns parse results for use with Execute.
func (m *Manager) Parse(ctx context.Context, src Source, command string) *ParseResults {
	return m.ParseReader(ctx, src, &brigodier.StringReader{String: command})
}

// ParseReader stores a required command invoker Source in ctx,
// parses the command and returns parse results for use with Execute.
func (m *Manager) ParseReader(ctx context.Context, src Source, command *brigodier.StringReader) *ParseResults {
	ctx = context.WithValue(ctx, sourceCtxKey, src)
	return (*ParseResults)(m.Dispatcher.ParseReader(ctx, command))
}

var ErrForward = errors.New("forward command")

// Do does a Parse and Execute.
func (m *Manager) Do(ctx context.Context, src Source, command string) error {
	return m.Execute(m.Parse(ctx, src, command))
}

// Execute ensures parse context has a Source and executes it.
func (m *Manager) Execute(parse *ParseResults) error {
	if SourceFromContext(parse.Context) == nil {
		return errors.New("context misses command source")
	}
	return m.Dispatcher.Execute((*brigodier.ParseResults)(parse))
}

// Has indicates whether the specified command/alias is registered.
func (m *Manager) Has(command string) bool {
	_, ok := m.Dispatcher.Root.Children()[strings.ToLower(command)]
	return ok
}

// CompletionSuggestions returns completion suggestions.
func (m *Manager) CompletionSuggestions(parse *ParseResults) (*brigodier.Suggestions, error) {
	return m.Dispatcher.CompletionSuggestions((*brigodier.ParseResults)(parse))
}

// OfferSuggestions returns completion suggestions.
func (m *Manager) OfferSuggestions(ctx context.Context, source Source, cmdline string) ([]string, error) {
	suggestions, err := m.CompletionSuggestions(m.Parse(ctx, source, cmdline))
	if err != nil {
		return nil, err
	}
	s := make([]string, 0, len(suggestions.Suggestions))
	for _, suggestion := range suggestions.Suggestions {
		s = append(s, suggestion.Text)
	}
	return s, nil
}

type sourceCtx struct{}

var sourceCtxKey = &sourceCtx{}
