package proxy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type CommandManager struct {
	mu       sync.RWMutex
	commands map[string]*registration
}

// newCommandManager returns a new CommandManager.
func newCommandManager() *CommandManager {
	return &CommandManager{commands: map[string]*registration{}}
}

type registration struct {
	cmd     Command
	aliases []string
}

// Register registers (and overrides) a command with the root literal name and optional aliases.
func (m *CommandManager) Register(cmd Command, name string, aliases ...string) {
	if cmd == nil {
		return
	}
	r := &registration{
		cmd:     cmd,
		aliases: append(aliases, name),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[name] = r
	for _, name := range aliases {
		m.commands[name] = r
	}
}

// Unregister unregisters a command with its aliases.
func (m *CommandManager) Unregister(name string) {
	m.mu.Lock()
	r, ok := m.commands[name]
	if ok {
		for _, name := range r.aliases {
			delete(m.commands, name)
		}
	}
	delete(m.commands, name)
	m.mu.Unlock()
}

// Has return true if the command is registered.
func (m *CommandManager) Has(command string) bool {
	m.mu.RLock()
	_, ok := m.commands[command]
	m.mu.RUnlock()
	return ok
}

// Invoke invokes a registered command.
func (m *CommandManager) Invoke(ctx *Context, command string) (found bool, err error) {
	if len(command) == 0 {
		return false, errors.New("command must not be empty")
	}
	if ctx == nil {
		return false, errors.New("ctx must not be nil")
	}
	if ctx.Source == nil {
		return false, errors.New("ctx source must not be nil")
	}
	if ctx.Context == nil {
		ctx.Context = context.Background()
	}
	m.mu.RLock()
	r, ok := m.commands[command]
	m.mu.RUnlock()
	if !ok {
		return false, nil
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic while invoking command: %v", r)
		}
	}()
	r.cmd.Invoke(ctx)
	return true, err
}

// Command is an invokable command.
type Command interface {
	Invoke(*Context)
}

// CommandFunc is a shorthand type that implements the Command interface.
type CommandFunc func(*Context)

// Invoke implements Command.
func (f CommandFunc) Invoke(c *Context) {
	f(c)
}

// Context is a command invocation context.
type Context struct {
	context.Context               // The context to propagate to subprocesses of the command invocation.
	Source          CommandSource // The one executing the command.
	Args            []string      // The command arguments (without the "/<command>" part)
}

var spaceRegex = regexp.MustCompile(`\s+`)

// trimSpaces removes all spaces that are to much.
func trimSpaces(s string) string {
	s = strings.TrimSpace(s)
	return spaceRegex.ReplaceAllString(s, " ") // remove to much spaces in between
}

func extract(commandline string) (command string, args []string, ok bool) {
	split := strings.Split(commandline, " ")
	if len(split) != 0 {
		command = split[0]
		ok = true
	}
	if len(split) > 1 {
		args = split[1:]
	}
	return
}
