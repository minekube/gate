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
