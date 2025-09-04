---
title: 'Gate Command System - Create Custom Commands'
description: 'Learn how to create custom commands for Gate Minecraft proxy with aliases, permissions, and argument handling using brigadier.'
---

# Command System

Gate provides a powerful command system based on Mojang's brigadier library, allowing you to create custom commands with full tab completion, argument validation, and permission support.

## Basic Command Registration

### Simple Command

:::: code-group

```go [Basic Command (Recommended)]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    // Register a simple command using command.Command wrapper
    p.proxy.Command().Register(
        brigodier.Literal("hello").
            Executes(command.Command(func(c *command.Context) error {
                // Source is directly available in command.Context
                c.Source.SendMessage(&Text{Content: "Hello, world!"})
                return nil
            })),
    )
}
```

```go [Legacy Approach]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    // Legacy approach with type assertion (not recommended)
    p.proxy.Command().Register(
        brigodier.Literal("hello").
            Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                source := c.Source.(command.Source)
                source.SendMessage(&Text{Content: "Hello, world!"})
                return nil
            })),
    )
}
```

```go [Command with Permission]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/util/permission"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("admin").
            Requires(command.Requires(func(c *command.RequiresContext) bool {
                return c.Source.PermissionValue("myplugin.admin") == permission.True
            })).
            Executes(command.Command(func(c *command.Context) error {
                c.Source.SendMessage(&Text{Content: "Admin command executed!"})
                return nil
            })),
    )
}
```

::::

### Command with Arguments

:::: code-group

```go [String Argument]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("say").
            Then(
                brigodier.Argument("message", brigodier.String()).
                    Executes(command.Command(func(c *command.Context) error {
                        message := brigodier.GetString(c.CommandContext, "message")
                        c.Source.SendMessage(&Text{Content: "You said: " + message})
                        return nil
                    })),
            ),
    )
}
```

```go [Player Argument]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/edition/java/proxy"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("teleport").
            Then(
                brigodier.Argument("player", brigodier.String()).
                    Suggests(playerSuggestionProvider(p.proxy)).
                    Executes(command.Command(func(c *command.Context) error {
                        playerName := brigodier.GetString(c.CommandContext, "player")

                        player := p.proxy.Player(playerName)
                        if player == nil {
                            c.Source.SendMessage(&Text{Content: "Player not found!"})
                            return nil
                        }

                        // Teleport logic here
                        c.Source.SendMessage(&Text{Content: "Teleported to " + playerName})
                        return nil
                    })),
            ),
    )
}

func playerSuggestionProvider(proxy *proxy.Proxy) brigodier.SuggestionProvider {
    return brigodier.SuggestionProviderFunc(func(
        ctx *brigodier.CommandContext,
        builder *brigodier.SuggestionsBuilder,
    ) *brigodier.Suggestions {
        for _, player := range proxy.Players() {
            builder.Suggest(player.Username())
        }
        return builder.Build()
    })
}
```

::::

## Command Aliases

Gate supports command aliases using the `RegisterWithAliases` method, allowing you to register a command with multiple names.

:::: code-group

```go [Single Alias]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    // Register command with alias
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("gamemode").
            Then(
                brigodier.Argument("mode", brigodier.String()).
                    Executes(command.Command(func(c *command.Context) error {
                        mode := brigodier.GetString(c.CommandContext, "mode")
                        c.Source.SendMessage(&Text{Content: "Changed gamemode to " + mode})
                        return nil
                    })),
            ),
        "gm", // Single alias
    )
}
```

```go [Multiple Aliases]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func (p *SimpleProxy) registerCommands() {
    // Register command with multiple aliases
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("teleport").
            Then(
                brigodier.Argument("player", brigodier.String()).
                    Executes(command.Command(func(c *command.Context) error {
                        player := brigodier.GetString(c.CommandContext, "player")
                        c.Source.SendMessage(&Text{Content: "Teleported to " + player})
                        return nil
                    })),
            ),
        "tp", "tele", // Multiple aliases
    )
}
```

```go [Complex Command with Aliases]
func (p *SimpleProxy) registerCommands() {
    // Complex command with subcommands and aliases
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("server").
            Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                // Show current server
                source := c.Source.(command.Source)
                if player, ok := source.(Player); ok {
                    current := player.CurrentServer()
                    if current != nil {
                        source.SendMessage(&Text{Content: "Current server: " + current.ServerInfo().Name()})
                    } else {
                        source.SendMessage(&Text{Content: "Not connected to any server"})
                    }
                }
                return nil
            })).
            Then(
                brigodier.Argument("server", brigodier.String()).
                    Suggests(serverSuggestionProvider(p.proxy)).
                    Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                        // Connect to server
                        source := c.Source.(command.Source)
                        serverName := brigodier.GetString(c, "server")

                        if player, ok := source.(Player); ok {
                            server := p.proxy.Server(serverName)
                            if server == nil {
                                source.SendMessage(&Text{Content: "Server not found!"})
                                return nil
                            }

                            player.CreateConnectionRequest(server).ConnectWithIndication()
                        }
                        return nil
                    })),
            ),
        []string{"connect", "join"}, // Aliases
    )
}
```

::::

## Advanced Features

### Permission-Based Commands

:::: code-group

```go [Role-Based Permissions]
func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("kick").
            Requires(func(source brigodier.CommandSource) bool {
                cmdSource := source.(command.Source)
                return cmdSource.PermissionValue("myplugin.kick") == permission.True
            }).
            Then(
                brigodier.Argument("player", brigodier.String()).
                    Then(
                        brigodier.Argument("reason", brigodier.GreedyString()).
                            Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                                source := c.Source.(command.Source)
                                playerName := brigodier.GetString(c, "player")
                                reason := brigodier.GetString(c, "reason")

                                player := p.proxy.Player(playerName)
                                if player == nil {
                                    source.SendMessage(&Text{Content: "Player not found!"})
                                    return nil
                                }

                                player.Disconnect(&Text{Content: "Kicked: " + reason})
                                source.SendMessage(&Text{Content: "Kicked " + playerName})
                                return nil
                            })),
                    ),
            ),
        []string{"boot"}, // Alias
    )
}
```

```go [Conditional Commands]
func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("debug").
            Requires(func(source brigodier.CommandSource) bool {
                cmdSource := source.(command.Source)
                // Only available to console or ops
                if _, isConsole := cmdSource.(command.ConsoleCommandSource); isConsole {
                    return true
                }
                return cmdSource.PermissionValue("myplugin.debug") == permission.True
            }).
            Then(
                brigodier.Literal("info").
                    Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                        source := c.Source.(command.Source)
                        info := fmt.Sprintf("Players: %d, Servers: %d",
                            len(p.proxy.Players()), len(p.proxy.Servers()))
                        source.SendMessage(&Text{Content: info})
                        return nil
                    })),
            ),
    )
}
```

::::

### Custom Argument Types

:::: code-group

```go [Server Argument]
func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("send").
            Then(
                brigodier.Argument("player", brigodier.String()).
                    Suggests(playerSuggestionProvider(p.proxy)).
                    Then(
                        brigodier.Argument("server", brigodier.String()).
                            Suggests(serverSuggestionProvider(p.proxy)).
                            Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                                source := c.Source.(command.Source)
                                playerName := brigodier.GetString(c, "player")
                                serverName := brigodier.GetString(c, "server")

                                player := p.proxy.Player(playerName)
                                server := p.proxy.Server(serverName)

                                if player == nil {
                                    source.SendMessage(&Text{Content: "Player not found!"})
                                    return nil
                                }
                                if server == nil {
                                    source.SendMessage(&Text{Content: "Server not found!"})
                                    return nil
                                }

                                player.CreateConnectionRequest(server).ConnectWithIndication()
                                source.SendMessage(&Text{Content: "Sent " + playerName + " to " + serverName})
                                return nil
                            })),
                    ),
            ),
    )
}

func serverSuggestionProvider(proxy *proxy.Proxy) brigodier.SuggestionProvider {
    return brigodier.SuggestionProviderFunc(func(
        ctx *brigodier.CommandContext,
        builder *brigodier.SuggestionsBuilder,
    ) *brigodier.Suggestions {
        for _, server := range proxy.Servers() {
            builder.Suggest(server.ServerInfo().Name())
        }
        return builder.Build()
    })
}
```

```go [Integer Argument with Validation]
func (p *SimpleProxy) registerCommands() {
    p.proxy.Command().Register(
        brigodier.Literal("setslots").
            Requires(func(source brigodier.CommandSource) bool {
                cmdSource := source.(command.Source)
                return cmdSource.PermissionValue("myplugin.setslots") == permission.True
            }).
            Then(
                brigodier.Argument("slots", brigodier.IntegerWithMin(1)).
                    Executes(brigodier.Command(func(c *brigodier.CommandContext) error {
                        source := c.Source.(command.Source)
                        slots := brigodier.GetInteger(c, "slots")

                        if slots > 1000 {
                            source.SendMessage(&Text{Content: "Maximum 1000 slots allowed!"})
                            return nil
                        }

                        // Update server slots logic here
                        source.SendMessage(&Text{Content: fmt.Sprintf("Set server slots to %d", slots)})
                        return nil
                    })),
            ),
    )
}
```

::::

## Command Manager Methods

The `command.Manager` provides several public methods for managing commands. Here are examples of all available methods:

:::: code-group

```go [Parse & Execute]
import (
    "context"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func handleCommand(mgr *command.Manager, source command.Source, cmdline string) {
    ctx := context.Background()

    // Parse command input
    parseResults := mgr.Parse(ctx, source, cmdline)

    // Execute parsed command
    if err := mgr.Execute(parseResults); err != nil {
        source.SendMessage(&Text{Content: "Command failed: " + err.Error()})
    }
}

// Or use Do() for parse + execute in one call
func handleCommandSimple(mgr *command.Manager, source command.Source, cmdline string) {
    ctx := context.Background()
    if err := mgr.Do(ctx, source, cmdline); err != nil {
        source.SendMessage(&Text{Content: "Command failed: " + err.Error()})
    }
}
```

```go [Command Existence & Suggestions]
import (
    "context"
    "strings"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func checkAndSuggest(mgr *command.Manager, source command.Source, input string) {
    // Check if command exists
    if mgr.Has("teleport") {
        source.SendMessage(&Text{Content: "Teleport command is available"})
    }

    // Get completion suggestions as strings
    ctx := context.Background()
    suggestions, err := mgr.OfferSuggestions(ctx, source, input)
    if err == nil && len(suggestions) > 0 {
        source.SendMessage(&Text{Content: "Suggestions: " + strings.Join(suggestions, ", ")})
    }

    // Get brigadier suggestions for more control
    brigSuggestions, err := mgr.OfferBrigodierSuggestions(ctx, source, input)
    if err == nil {
        for _, suggestion := range brigSuggestions.Suggestions {
            source.SendMessage(&Text{Content: "Suggestion: " + suggestion.Text})
        }
    }
}
```

```go [Advanced Parsing]
import (
    "context"
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func advancedParsing(mgr *command.Manager, source command.Source, input string) {
    ctx := context.Background()

    // Parse using StringReader for more control
    reader := &brigodier.StringReader{String: input}
    parseResults := mgr.ParseReader(ctx, source, reader)

    // Get completion suggestions for parsed results
    suggestions, err := mgr.CompletionSuggestions(parseResults)
    if err == nil {
        for _, suggestion := range suggestions.Suggestions {
            source.SendMessage(&Text{Content: "Completion: " + suggestion.Text})
        }
    }

    // Execute the parsed command
    if err := mgr.Execute(parseResults); err != nil {
        source.SendMessage(&Text{Content: "Execution failed: " + err.Error()})
    }
}
```

::::

### Context Helpers

The command package provides helper functions for working with command contexts:

:::: code-group

```go [Context Helpers]
import (
    "context"
    "go.minekube.com/gate/pkg/command"
    . "go.minekube.com/common/minecraft/component"
)

func contextExamples() {
    ctx := context.Background()

    // Create a mock source (in real usage, this comes from Gate)
    var source command.Source

    // Add source to context
    ctxWithSource := command.ContextWithSource(ctx, source)

    // Retrieve source from context
    retrievedSource := command.SourceFromContext(ctxWithSource)
    if retrievedSource != nil {
        retrievedSource.SendMessage(&Text{Content: "Source retrieved successfully"})
    }
}
```

```go [Command Wrappers]
import (
    "go.minekube.com/brigodier"
    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/util/permission"
    . "go.minekube.com/common/minecraft/component"
)

func wrapperExamples() {
    // Use command.Command wrapper for cleaner code
    cmd := command.Command(func(c *command.Context) error {
        // Direct access to source, no type assertion needed
        c.Source.SendMessage(&Text{Content: "Hello from wrapped command!"})

        // Access brigadier context for arguments
        if arg := brigodier.GetString(c.CommandContext, "argument"); arg != "" {
            c.Source.SendMessage(&Text{Content: "Argument: " + arg})
        }

        return nil
    })

    // Use command.Requires wrapper for permission checks
    requiresFn := command.Requires(func(c *command.RequiresContext) bool {
        // Direct access to source for permission checking
        return c.Source.PermissionValue("myplugin.use") == permission.True
    })

    // These can be used in brigadier builders
    _ = brigodier.Literal("example").
        Requires(requiresFn).
        Executes(cmd)
}
```

::::

## Complete Example

Here's a complete example showing a plugin with multiple commands and aliases:

:::: code-group

```go [Plugin Structure]
package main

import (
    "context"
    "fmt"

    "go.minekube.com/brigodier"
    "go.minekube.com/common/minecraft/component"
    . "go.minekube.com/common/minecraft/component"
    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/edition/java/proxy"
    "go.minekube.com/gate/pkg/gate"
    "go.minekube.com/gate/pkg/util/permission"
)

type MyPlugin struct {
    proxy *proxy.Proxy
}

func main() {
    proxy.Plugins = append(proxy.Plugins, proxy.Plugin{
        Name: "MyPlugin",
        Init: func(ctx context.Context, proxy *proxy.Proxy) error {
            plugin := &MyPlugin{proxy: proxy}
            plugin.registerCommands()
            return nil
        },
    })

    gate.Execute()
}
```

```go [Command Registration]
func (p *MyPlugin) registerCommands() {
    // Simple command with alias
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("ping").
            Executes(command.Command(func(c *command.Context) error {
                c.Source.SendMessage(&Text{Content: "Pong!"})
                return nil
            })),
        "pong",
    )

    // Server management command with multiple aliases
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("server").
            Executes(command.Command(p.showCurrentServer)).
            Then(
                brigodier.Argument("server", brigodier.String()).
                    Suggests(p.serverSuggestions()).
                    Executes(command.Command(p.connectToServer)),
            ).
            Then(
                brigodier.Literal("list").
                    Executes(command.Command(p.listServers)),
            ),
        "connect", "join", "switch",
    )

    // Admin command with permission check
    p.proxy.Command().RegisterWithAliases(
        brigodier.Literal("admin").
            Requires(command.Requires(func(c *command.RequiresContext) bool {
                return c.Source.PermissionValue("myplugin.admin") == permission.True
            })).
            Then(
                brigodier.Literal("reload").
                    Executes(command.Command(p.reloadConfig)),
            ).
            Then(
                brigodier.Literal("info").
                    Executes(command.Command(p.showInfo)),
            ),
        "manage",
    )
}
```

```go [Command Implementations]
func (p *MyPlugin) showCurrentServer(c *command.Context) error {
    player, ok := c.Source.(proxy.Player)
    if !ok {
        return c.Source.SendMessage(&Text{Content: "This command is only available to players"})
    }

    current := player.CurrentServer()
    if current != nil {
        return c.Source.SendMessage(&Text{Content: "Current server: " + current.ServerInfo().Name()})
    }
    return c.Source.SendMessage(&Text{Content: "Not connected to any server"})
}

func (p *MyPlugin) connectToServer(c *command.Context) error {
    serverName := brigodier.GetString(c.CommandContext, "server")

    player, ok := c.Source.(proxy.Player)
    if !ok {
        return c.Source.SendMessage(&Text{Content: "This command is only available to players"})
    }

    server := p.proxy.Server(serverName)
    if server == nil {
        return c.Source.SendMessage(&Text{Content: "Server '" + serverName + "' not found!"})
    }

    player.CreateConnectionRequest(server).ConnectWithIndication()
    return c.Source.SendMessage(&Text{Content: "Connecting to " + serverName + "..."})
}

func (p *MyPlugin) listServers(c *command.Context) error {
    servers := p.proxy.Servers()

    if len(servers) == 0 {
        return c.Source.SendMessage(&Text{Content: "No servers configured"})
    }

    c.Source.SendMessage(&Text{Content: "Available servers:"})
    for _, server := range servers {
        c.Source.SendMessage(&Text{Content: "- " + server.ServerInfo().Name()})
    }
    return nil
}

func (p *MyPlugin) reloadConfig(c *command.Context) error {
    // Reload configuration logic here
    return c.Source.SendMessage(&Text{Content: "Configuration reloaded!"})
}

func (p *MyPlugin) showInfo(c *command.Context) error {
    playerCount := len(p.proxy.Players())
    serverCount := len(p.proxy.Servers())

    return c.Source.SendMessage(&Text{Content: fmt.Sprintf("Players: %d, Servers: %d", playerCount, serverCount)})
}

func (p *MyPlugin) serverSuggestions() brigodier.SuggestionProvider {
    return brigodier.SuggestionProviderFunc(func(
        ctx *brigodier.CommandContext,
        builder *brigodier.SuggestionsBuilder,
    ) *brigodier.Suggestions {
        for _, server := range p.proxy.Servers() {
            builder.Suggest(server.ServerInfo().Name())
        }
        return builder.Build()
    })
}
```

::::

## Working with Players

When creating commands that need to interact with players specifically, you'll need to check if the command source is a player. Here are the proper patterns:

:::: code-group

```go [Player Type Assertion (Recommended)]
import (
    "fmt"

    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/edition/java/proxy"
    . "go.minekube.com/common/minecraft/component"
)

func playerOnlyCommand(c *command.Context) error {
    // Always use !ok pattern for early return
    player, ok := c.Source.(proxy.Player)
    if !ok {
        return c.Source.SendMessage(&Text{Content: "This command is only available to players"})
    }

    // Now you can safely use player methods
    return player.SendMessage(&Text{
        Content: fmt.Sprintf("Hello %s! Your ping is %s",
            player.Username(),
            player.Ping()),
    })
}
```

```go [Mixed Source Command]
import (
    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/edition/java/proxy"
    . "go.minekube.com/common/minecraft/component"
)

func mixedSourceCommand(c *command.Context) error {
    // Handle both players and non-player sources (console, etc.)
    if player, ok := c.Source.(proxy.Player); ok {
        // Player-specific logic
        current := player.CurrentServer()
        if current != nil {
            return c.Source.SendMessage(&Text{
                Content: "You are on: " + current.ServerInfo().Name(),
            })
        }
        return c.Source.SendMessage(&Text{Content: "Not connected to any server"})
    }

    // Non-player source logic (console, etc.)
    return c.Source.SendMessage(&Text{Content: "Command executed from non-player source"})
}
```

```go [Player Information Access]
import (
    "fmt"

    "go.minekube.com/gate/pkg/command"
    "go.minekube.com/gate/pkg/edition/java/proxy"
    . "go.minekube.com/common/minecraft/component"
)

func playerInfoCommand(c *command.Context) error {
    player, ok := c.Source.(proxy.Player)
    if !ok {
        return c.Source.SendMessage(&Text{Content: "Player-only command"})
    }

    // Access player information
    info := fmt.Sprintf(`Player Info:
- Username: %s
- UUID: %s
- Ping: %s
- Protocol: %d
- Connected: %t`,
        player.Username(),
        player.ID(),
        player.Ping(),
        player.Protocol().Protocol,
        player.Active(),
    )

    return c.Source.SendMessage(&Text{Content: info})
}
```

::::

### Player vs Non-Player Commands

- **Player-only commands**: Use `player, ok := c.Source.(proxy.Player); if !ok { return ... }`
- **Non-player sources**: When `c.Source` is not a `proxy.Player`, it could be console or other command sources
- **Mixed commands**: Handle both cases with appropriate logic for each

**Note**: There is no specific `ConsoleCommandSource` type. Console commands simply implement the `command.Source` interface but are not `proxy.Player` instances.

## Key Features

- **Command Aliases**: Use `RegisterWithAliases()` to create multiple names for the same command
- **Permission System**: Integrate with Gate's permission system using `Requires()`
- **Tab Completion**: Provide suggestions using `Suggests()` with custom providers
- **Argument Validation**: Use brigadier's built-in argument types with validation
- **Player-Only Commands**: Check if source is a player before executing player-specific logic
- **Error Handling**: Return errors from command execution for proper error reporting

## Best Practices

1. **Use `command.Command` Wrapper**: Always use `command.Command(func(c *command.Context) error)` instead of raw brigadier commands for cleaner code
2. **Player Type Assertions**: Use `player, ok := c.Source.(proxy.Player); if !ok { return ... }` pattern for safety
3. **Early Returns**: Return errors immediately from `SendMessage()` calls for cleaner code flow
4. **Use Aliases Wisely**: Provide common abbreviations (e.g., `tp` for `teleport`, `gm` for `gamemode`)
5. **Permission Checks**: Use `command.Requires()` wrapper for permission validation
6. **Input Validation**: Validate arguments before processing
7. **User Feedback**: Provide clear success/error messages
8. **Tab Completion**: Implement suggestion providers for better user experience
9. **Documentation**: Document your commands and their usage

## Related Features

- **[Events System](/developers/events)** - Handle player and server events
- **[Permission System](/guide/config/#permissions)** - Configure player permissions
- **[Built-in Commands](/guide/builtin-commands)** - Gate's default commands
