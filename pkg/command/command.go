package command

import (
	"context"
	"errors"
	"strings"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/permission"
)

// Manager is a command manager for
// registering and executing proxy commands.
type Manager struct{ brigodier.Dispatcher }

// Source is the invoker of a command.
// It could be a player or the console/terminal.
type Source interface {
	permission.Subject
	// SendMessage sends a message component to the invoker.
	SendMessage(msg component.Component, opts ...MessageOption) error
}

type MessageOption interface {
	Apply(o any)
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

// ContextWithSource returns a new context including the specified Source.
func ContextWithSource(ctx context.Context, src Source) context.Context {
	return context.WithValue(ctx, sourceCtxKey, src)
}

// Context wraps the context for a brigodier.Command.
type Context struct {
	*brigodier.CommandContext
	Source
}

func createContext(c *brigodier.CommandContext) *Context {
	return &Context{
		CommandContext: c,
		Source:         SourceFromContext(c),
	}
}

// RequiresContext wraps the context for a brigodier.RequireFn.
type RequiresContext struct {
	context.Context
	Source
}

// Command wraps the context for a brigodier.Command.
func Command(fn func(c *Context) error) brigodier.Command {
	return brigodier.CommandFunc(func(c *brigodier.CommandContext) error {
		return fn(createContext(c))
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
	ctx = ContextWithSource(ctx, src)
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

// RegisterWithAliases registers a command with multiple aliases using shallow copy approach.
// This is based on Velocity's implementation to avoid brigadier redirect limitations.
func (m *Manager) RegisterWithAliases(command brigodier.LiteralNodeBuilder, aliases ...string) *brigodier.LiteralCommandNode {
	// Register the primary command
	primary := m.Register(command)

	// Create aliases using shallow copy approach (like Velocity)
	for _, alias := range aliases {
		aliasNode := m.shallowCopy(primary, strings.ToLower(alias))
		m.Root.AddChild(aliasNode)
	}

	return primary
}

// shallowCopy creates a shallow copy of a command node with a new name.
// This implementation is based on Velocity's shallowCopy method which avoids
// brigadier redirect limitations with suggestions (Mojang/brigadier#46).
func (m *Manager) shallowCopy(original *brigodier.LiteralCommandNode, newName string) *brigodier.LiteralCommandNode {
	// Create new literal builder with the alias name - chain calls to avoid type assertion issues
	var builder brigodier.LiteralNodeBuilder = brigodier.Literal(newName)

	// Copy requirement if it exists
	if original.Requirement() != nil {
		builder = builder.Requires(original.Requirement())
	}

	// Copy execution command if it exists
	if original.Command() != nil {
		builder = builder.Executes(original.Command())
	}

	// Copy redirect information if it exists
	if original.Redirect() != nil {
		if original.RedirectModifier() != nil {
			builder = builder.RedirectWithModifier(original.Redirect(), original.RedirectModifier())
		} else {
			builder = builder.Redirect(original.Redirect())
		}
		if original.IsFork() {
			builder = builder.Fork(original.Redirect(), original.RedirectModifier())
		}
	}

	// Build the node first
	aliasNode := builder.BuildLiteral()

	// Copy all children (shallow copy)
	for _, child := range original.Children() {
		aliasNode.AddChild(child)
	}

	return aliasNode
}

// OfferSuggestions returns completion suggestions.
func (m *Manager) OfferSuggestions(ctx context.Context, source Source, cmdline string) ([]string, error) {
	suggestions, err := m.OfferBrigodierSuggestions(ctx, source, cmdline)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(suggestions.Suggestions))
	for i, s := range suggestions.Suggestions {
		result[i] = s.Text
	}
	return result, nil
}

// OfferBrigodierSuggestions returns brigodier Suggestions for the given command line.
func (m *Manager) OfferBrigodierSuggestions(ctx context.Context, source Source, cmdline string) (*brigodier.Suggestions, error) {
	cmdLine := normalizeInput(cmdline, false)
	suggestions, err := m.CompletionSuggestions(m.Parse(ctx, source, cmdLine))
	if err != nil {
		return nil, err
	}
	return suggestions, nil
}

type sourceCtx struct{}

var sourceCtxKey = &sourceCtx{}

// normalizeInput normalizes the given command input.
// input: the raw command input, without the leading slash ('/')
// trim: whether to remove leading and trailing whitespace from the input
// returns the normalized command input
func normalizeInput(input string, trim bool) string {
	command := input
	if trim {
		command = strings.TrimSpace(input)
	}
	firstSep := strings.IndexRune(command, brigodier.ArgumentSeparator)
	if firstSep != -1 {
		// Aliases are case-insensitive, arguments are not
		return strings.ToLower(command[:firstSep]) + command[firstSep:]
	}
	return strings.ToLower(command)
}
