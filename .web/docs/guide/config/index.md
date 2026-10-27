---
title: 'Gate Configuration Guide - Proxy Settings'
description: 'Configure Gate Minecraft proxy with YAML settings, server routes, authentication, and advanced proxy features.'
---

# Configuration

Gate uses a YAML configuration file (`config.yml`) to configure all proxy settings.

## Config File Location

Gate looks for `config.yml` in the current working directory by default. Use `--config` or `-c` to specify a custom path:

```sh console
$ gate                    # Uses ./config.yml
$ gate -c /path/to/config.yml
```

Gate supports YAML (`.yml`, `.yaml`), JSON (`.json`), and environment variables with `GATE_` prefix. You can mix formats - use a config file and override values with environment variables.

## Running Without a Config File

Gate starts on its built-in defaults when it finds no config file, and logs a hint saying so:

```
no config file found, using built-in defaults	{"hint": "run `gate config --write` to generate one, or start Gate with --init-config"}
```

If you want the file to exist without writing it by hand, let Gate create it on start:

```sh console
$ gate --init-config
```

With `--init-config` (or `GATE_INIT_CONFIG=true` in a container), Gate writes `config.yml` when it found no config file, and then reads it - so this start already uses the file, and the auto config reload watches it from here on. Two rules hold:

- An existing config file is never overwritten. If Gate finds one - `./config.yml` or the path given with `--config` - it runs on that file and writes nothing.
- The generated file states only default values, so generating it changes nothing about how Gate behaves. Add your `servers` and `try` list to it, or generate an annotated template instead (see below).

If the working directory is not writable, Gate logs the failure and keeps running on the built-in defaults.

## Configuration Templates

Generate configuration files using the `gate config` command, or create them manually:

```sh console
# Write to config.yml
$ gate config --write
$ gate config --type <type> --write

# Write to custom file using pipe redirection
$ gate config > my-config.yml
$ gate config --type <type> > my-config.yml
```

`--write` never overwrites an existing `config.yml`: it fails with a message naming the file, and `--force` is the explicit way to replace it. Pipe redirection is a plain shell redirect and always replaces the target file.

You can also create the `config.yml` file manually using any of the templates below as a starting point.

::: code-group

```yaml [Full (default)]
<!--@include: ../../../../config.yml -->
```

```yaml [Simple]
<!--@include: ../../../../config-simple.yml -->
```

```yaml [Lite]
<!--@include: ../../../../config-lite.yml -->
```

```yaml [Bedrock]
<!--@include: ../../../../config-bedrock.yml -->
```

```yaml [Minimal]
<!--@include: ../../../../pkg/configs/config-minimal.yml -->
```

:::

For most users, the full configuration is recommended. You can generate it and then edit the `servers` section to point to your backend Minecraft servers.
