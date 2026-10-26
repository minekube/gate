---
title: "Gate Troubleshooting & Diagnostics"
description: "Diagnose a hung or stalled Gate proxy: how to capture a goroutine dump that actually prints stacks, why kill -QUIT only shuts the proxy down, and what to send to support."
---

# Troubleshooting & Diagnostics

This page collects the operator-facing diagnostics for a Gate proxy that is
running but not behaving - a join that never completes, a connection that stops
moving. Start with the console output; if the proxy looks stuck, a goroutine dump
shows exactly where every goroutine is waiting.

## Keeping logs across restarts

Gate writes its log to **stderr** and has no log-file setting of its own, so what survives a restart is
whatever recorded the stream. Redirect it into a file, or let the platform keep it:

```sh
# Bare binary: append, and still watch the console
./gate 2>&1 | tee -a gate.log

# Or redirect only, e.g. from a start script
./gate >> gate.log 2>&1
```

- **systemd**: keep it in the journal (`journalctl -u gate`) or in a file with
  `StandardOutput=append:/var/log/gate/gate.log` and `StandardError=append:/var/log/gate/gate.log` in the unit.
- **Docker**: use a log driver that writes files (`--log-driver local`, or `json-file` with
  `--log-opt max-size=...`), then read it with `docker logs`. A container's log dies with the container
  unless the driver stores it.
- **Kubernetes**: `kubectl logs` only returns the **current** container instance, and `--previous` only the
  one before it. Ship the stream to your logging stack (OTel Collector, Promtail, your cloud's agent) or
  mount a volume and redirect the process output into it.

### Colour codes in a redirected log

By default Gate colours the log level, and a redirected file is not a terminal, so the file fills up with
ANSI escapes (`\x1b[32mINFO\x1b[0m ...`) that every editor and log pipeline has to strip. Set `NO_COLOR`
([no-color.org](https://no-color.org)) or `GATE_NO_COLOR` to plain text:

```sh
NO_COLOR=1 ./gate >> gate.log 2>&1
```

An empty value or `0` keeps colour, so `NO_COLOR=0` does not disable it.

Gate does not rotate files itself - use `logrotate`, journald, or the container log driver for that.

## Getting a goroutine dump from a hung Gate

Gate is a Go program, so a running Gate can print the stack of every goroutine it
has. The signal people reach for is `SIGQUIT`, because that is what a Go runtime
dumps on by default - **it does not work on Gate**: SIGHUP, SIGINT, SIGTERM and
SIGQUIT are all registered as termination signals, so Gate handles `SIGQUIT`
itself and `os/signal` never lets the Go runtime see it. `kill -QUIT` therefore
shuts the proxy down gracefully and prints **no stacks at all**.

The signal that still produces a dump is `SIGABRT`:

```sh
pgrep -f gate_0.74      # find the Gate process id, e.g. 1194
kill -ABRT <pid>        # print every goroutine stack, then end the run
```

What to expect:

- First line `SIGABRT: abort`, then a `PC=0x… m=0 sigcode=0` line.
- One `goroutine <n> [<state>]` block per goroutine, each with a full stack -
  roughly 30 kB of output (26 goroutines in our measurement of a freshly started
  0.74.x `linux/amd64` build).
- No environment variable is needed. `GOTRACEBACK=all` adds nothing here, and
  `kill -SEGV` behaves the same as `SIGABRT`.
- Only goroutines are dumped, so run it while the problem is happening.

::: warning `kill -ABRT` ends the Gate run
Capturing the dump *is* killing the process: players are disconnected, the exit
status is 2, and no graceful shutdown (and no shutdown reason) follows. Capture
the console **before** you send the signal - `docker logs -f <container>` or
`kubectl logs -f <pod>` (see [Docker](/guide/install/docker) and
[Kubernetes](/guide/install/kubernetes)) - because the dump is ~30 kB and a
container restart can push it out of a short log buffer. Then send the whole
output: support needs the stacks, not an excerpt.
:::

There is no debug endpoint or debugger below this: Gate does not serve
`net/http/pprof`, and release binaries are stripped (no DWARF, no ELF function
symbols), so a debugger attached with `gdb -p` cannot even list goroutines.

### Signals at a glance

| Signal | What Gate does |
| --- | --- |
| `SIGHUP`, `SIGINT`, `SIGTERM`, `SIGQUIT` | Graceful shutdown: logs `Received os signal {"signal": "quit"}`, disconnects players, exits 0. **No goroutine dump** - even for `SIGQUIT`. |
| `SIGABRT` | Dumps every goroutine stack, then exits 2. |
| `SIGSEGV` | Same dump as `SIGABRT`, then exits 2. |

A shutdown that *looks* like a dump in a pasted log - `Received os signal`
followed by `disconnecting all players` - is the graceful path, not a dump. If
you asked for a goroutine dump and see those lines, no stacks were produced, and
`kill -ABRT` is what you want instead. Newer Gate releases say so on the shutdown
line itself, next to the signal (source file and line vary by release):

```text
INFO Received os signal {"signal": "quit", "hint": "SIGQUIT is a graceful shutdown and prints no goroutine dump; for a goroutine dump use kill -ABRT <pid> (it ends the Gate run, so capture the console first)"}
```
