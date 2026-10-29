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

Gate logs to **stderr** by default, so on its own nothing survives a restart. There are two ways to keep
the log; pick one.

### Option 1: let Gate write the file

```yaml
config:
  log:
    # Directory that receives gate.log, created if missing. A relative path is
    # resolved against the working directory, so "logs" keeps the file next to a
    # proxy started from its own folder. Empty (the default) disables the file.
    directory: logs
    # Rotate the file once it grows past this size in megabytes; 0 disables rotation.
    maxSizeMB: 50
    # How many rotated files to keep, oldest deleted first; 0 keeps all of them.
    maxBackups: 5
```

The console log is unchanged: the file is written **in addition** to stderr, is appended to on every
start, and never contains colour codes. Rotated files are named `gate.<timestamp>.log` beside the active
`gate.log`. Rejecting an unreadable directory is a startup error rather than a silently missing log, and
changing these settings takes effect on restart.

### Option 2: let the platform record the stream

- **systemd**: keep it in the journal (`journalctl -u gate`) or in a file with
  `StandardOutput=append:/var/log/gate/gate.log` and `StandardError=append:/var/log/gate/gate.log` in the unit.
- **Docker**: use a log driver that writes files (`--log-driver local`, or `json-file` with
  `--log-opt max-size=...`), then read it with `docker logs`. A container's log dies with the container
  unless the driver stores it.
- **Kubernetes**: `kubectl logs` only returns the **current** container instance, and `--previous` only the
  one before it. Ship the stream to your logging stack (OTel Collector, Promtail, your cloud's agent) or
  mount a volume and redirect the process output into it.

Shell redirection (`./gate >> gate.log 2>&1`) still works for a quick local run, but the options above are
what keep the log across restarts without a wrapper script.

### Colour codes in a redirected log

Gate colours the log level on the console. A file - whether written by Gate or by shell redirection - is
not a terminal, so the redirected output keeps ANSI escapes (`\x1b[32mINFO\x1b[0m ...`) that editors and
log pipelines have to strip. Set `NO_COLOR` ([no-color.org](https://no-color.org)) or `GATE_NO_COLOR` for
plain text; an empty value or `0` keeps colour, so `NO_COLOR=0` does not disable it.

```sh
NO_COLOR=1 ./gate >> gate.log 2>&1
```

`config.log.directory` needs no such flag: its file is always plain text.

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
