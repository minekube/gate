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

# Write to custom file using pipe redirection
$ gate config > my-config.yml
```

You can also create the `config.yml` file manually using the template below as a starting point.

```yaml [Full]
<!--@include: ../../../../config.yml -->
```

For most users, the full configuration is recommended. You can generate it and then edit the `servers` section to point to your backend Minecraft servers.
