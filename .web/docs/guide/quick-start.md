# Quick Start

_This page quickly explains how to run Gate as a Minecraft proxy for your servers.
If you want to extend Gate with custom functionality, see the [Developers](/developers/) section._

---

<!--@include: install/index.md -->

## Running Gate

After installing the binary, you can run the Gate Minecraft proxy using the `gate` command.

```sh console
$ gate
INFO	gate/root.go:93	logging verbosity	{"verbosity": 0}
INFO	gate/root.go:94	using config file	{"config": ""}
INFO	config	gate/gate.go:205	config validation warn	{"warn": "java: No backend servers configured."}
INFO	java	proxy/proxy.go:299	Using favicon from data uri	{"length": 3086}
INFO	java	proxy/proxy.go:472	listening for connections	{"addr": "0.0.0.0:25565"}
```

::: tip Running Gate Lite Mode

Gate also has a [Lite mode](lite) that can passthrough connections based on the hostname.

:::

## Configuring Backend Servers

Gate connects to your Minecraft servers and forwards client connections to them.

You can do this by creating and editing the `config.yml` file.

```yaml config.yml
<!--@include: ../../../config-simple.yml -->
```

The `servers` section defines the addresses of your Minecraft servers.
and the `try` section defines the order in which players fallback to connect to.

There are many more options to configure, see [Configuration](/guide/config/) for more!

## Next Steps

<div class="next-steps">
  <a href="/guide/config/" class="next-card" style="text-decoration: none;">
    📖 Configuration Guide
    <span>Learn about all configuration options</span>
  </a>
  <a href="/developers/" class="next-card" style="text-decoration: none;">
    💻 Developer Guide
    <span>Extend Gate with custom code</span>
  </a>
  <a href="/guide/why" class="next-card" style="text-decoration: none;">
    🎯 Why Gate?
    <span>Learn about Gate's advantages</span>
  </a>
</div>

<style>
.quick-start-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
  margin: 24px 0;
}

.quick-card {
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  padding: 20px;
}

.quick-card h3 {
  margin-top: 0;
  color: var(--vp-c-brand-1);
}

.next-steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-top: 24px;
}

.next-card {
  padding: 16px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  text-decoration: none;
  color: var(--vp-c-brand-1);
  font-weight: 500;
  transition: all 0.3s;
}

.next-card:hover {
  transform: translateY(-2px);
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 2px 12px 0 var(--vp-c-divider);
}

.next-card span {
  display: block;
  color: var(--vp-c-text-2);
  font-size: 0.9em;
  font-weight: 400;
  margin-top: 4px;
}
</style>
