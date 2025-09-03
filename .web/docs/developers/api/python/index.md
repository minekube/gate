---
title: "Gate Python API - Minecraft Proxy Development in Python"
description: "Develop Minecraft proxy extensions with Gate Python API. Easy-to-use SDK with examples and integration guides."
---

# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/python/python-original.svg" class="tech-icon" alt="Python" /> Python Client

Gate provides a Python API for integrating with your Python applications. You can use the API to interact with Gate programmatically using either gRPC or HTTP protocols.

## Environment Setup

::: tip Best DX
We recommend using [uv](https://github.com/astral-sh/uv) for beginners and experienced developers alike!
:::

::: code-group

```bash [uv (Recommended)]
# Install uv (on macOS/Linux)
curl -LsSf https://astral.sh/uv/install.sh | sh

# Create a new virtual environment
uv init
```

```bash [venv]
# Create a new virtual environment
python3 -m venv .venv

# Activate the environment (Unix)
source .venv/bin/activate

# Activate the environment (Windows)
.venv\Scripts\activate
```

```bash [poetry]
# Install poetry
curl -sSL https://install.python-poetry.org | python3 -

# Create new project
poetry init
poetry shell
```

```bash [pipenv]
# Install pipenv
pip install pipenv

# Create environment and activate shell
pipenv install
pipenv shell
```

:::

## Installation

::: code-group

```bash [uv (Recommended)]
uv add minekube-gate-grpc-python minekube-gate-protocolbuffers-python --index https://buf.build/gen/python
```

```bash [pip]
python3 -m pip install minekube-gate-grpc-python minekube-gate-protocolbuffers-python --extra-index-url https://buf.build/gen/python
```

```bash [poetry]
poetry add minekube-gate-grpc-python minekube-gate-protocolbuffers-python --source buf.build/gen/python
```

```bash [pipenv]
pipenv install minekube-gate-grpc-python minekube-gate-protocolbuffers-python --extra-index-url https://buf.build/gen/python
```

:::

## Usage Example

Here's a basic example of using the Gate Python API to connect to Gate and list servers:

::: code-group

```python [main.py]
<!--@include: ./main.py -->
```

:::

## Running the Example

1. Make sure Gate is running with the API enabled
2. Save one of the example scripts above to `main.py`
3. Run the script:

::: code-group

```bash [uv (Recommended)]
uv run main.py
{
  "servers": [
    {
      "name": "server3",
      "address": "localhost:25568"
    },
    {
      "name": "server4",
      "address": "localhost:25569"
    },
    {
      "name": "server1",
      "address": "localhost:25566"
    },
    {
      "name": "server2",
      "address": "localhost:25567"
    }
  ]
}
```

```bash [python]
python3 main.py
```

```bash [poetry]
poetry run python main.py
```

```bash [pipenv]
pipenv run python main.py
```

:::

::: info Learn More

- [uv Documentation](https://github.com/astral-sh/uv) - Learn more about the recommended Python package manager
- [gRPC Python Documentation](https://grpc.io/docs/languages/python/basics/) - Learn more about using gRPC with Python
  :::

<style>
.tech-icon {
  width: 32px;
  height: 32px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 12px;
  position: relative;
  top: -2px;
}
</style>
