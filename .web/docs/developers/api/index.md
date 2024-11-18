# Gate HTTP API

Gate provides a powerful API that exposes its functionality to a wide ecosystem of languages and tools. Using modern technologies like Protocol Buffers, gRPC, and ConnectRPC with schemas managed through buf.build.

::: tip Quick Start
Simply enable the API in Gate's configuration, choose your preferred language's client library, and start building!
:::

::: code-group

```yaml [config.yml]
api:
  enabled: true
  bind: localhost:8080
```

:::

## Features

::: info Why Gate API?
The HTTP API enables you to build and deploy functionality independently from your proxy - perfect for rapid iteration without disrupting your players.
:::

<div class="vp-features">
  <div class="vp-feature">
    <div class="title">🚀 Independent Updates</div>
    <div class="details">Ship updates without restarting Gate or disconnecting players</div>
  </div>
  <div class="vp-feature">
    <div class="title">🌐 Cross-Language Support</div>
    <div class="details">Access Gate's core functionality from any programming language</div>
  </div>
  <div class="vp-feature">
    <div class="title">🔌 Plugin Development</div>
    <div class="details">Build extensions and plugins in your preferred language</div>
  </div>
  <div class="vp-feature">
    <div class="title">🤖 Automation</div>
    <div class="details">Automate server registration and management tasks</div>
  </div>
  <div class="vp-feature">
    <div class="title">🎮 Custom Tools</div>
    <div class="details">Create administrative interfaces and management tools</div>
  </div>
  <div class="vp-feature">
    <div class="title">🔄 Integration</div>
    <div class="details">Connect Gate with external systems and services</div>
  </div>
</div>

## Getting Started

::: warning Go Applications
While Go applications can use this HTTP API as well, we recommend using Gate's native Go library as it provides the most complete and type-safe access to Gate's functionality, unless you need out-of-process execution or want to iterate and deploy updates independently from your proxy.
:::

Gate's API definitions are hosted on [buf.build/minekube/gate](https://buf.build/minekube/gate/sdks), where you can directly pull client libraries using your preferred language's package manager:

<div class="vp-features">
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/typescript/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/typescript/typescript-original.svg" class="tech-icon" alt="TypeScript" />
        TypeScript
      </div>
      <div class="details">via npm packages</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/python/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/python/python-original.svg" class="tech-icon" alt="Python" />
        Python
      </div>
      <div class="details">via pip packages</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/java/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/java/java-original.svg" class="tech-icon" alt="Java" />
        Java
      </div>
      <div class="details">via Maven artifacts</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/go/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/go/go-original.svg" class="tech-icon" alt="Go" />
        Go
      </div>
      <div class="details">via go modules</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/rust/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/rust/rust-original.svg" class="tech-icon" alt="Rust" />
        Rust
      </div>
      <div class="details">via crates</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="/developers/api/kotlin/" class="feature-link">
      <div class="title">
        <img src="https://raw.githubusercontent.com/devicons/devicon/master/icons/kotlin/kotlin-original.svg" class="tech-icon" alt="Kotlin" />
        Kotlin
      </div>
      <div class="details">via Maven artifacts</div>
    </a>
  </div>
</div>

::: tip Learn More
To understand the key technologies used in Gate's API, check out the [Glossary](/developers/api/glossary).
:::

<style>
.vp-features {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
  margin: 20px 0;
}

.vp-feature {
  padding: 20px;
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  transition: all 0.3s;
}

.vp-feature:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 12px 0 var(--vp-c-divider);
  border-color: var(--vp-c-brand-1);
}

.vp-feature .title {
  font-size: 1.1em;
  font-weight: 600;
  margin-bottom: 8px;
  color: var(--vp-c-text-1);
}

.vp-feature .details {
  color: var(--vp-c-text-2);
  font-size: 0.9em;
  line-height: 1.4;
}

.vp-feature-small {
  padding: 12px;
  border-radius: 6px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  text-align: center;
  transition: all 0.3s;
}

.vp-feature-small:hover {
  border-color: var(--vp-c-brand-1);
  transform: translateY(-1px);
  box-shadow: 0 2px 8px 0 var(--vp-c-divider);
}

.vp-feature-small .title {
  font-weight: 600;
  margin-bottom: 4px;
  color: var(--vp-c-text-1);
  display: flex;
  align-items: center;
  justify-content: center;
}

.vp-feature-small .details {
  color: var(--vp-c-text-2);
  font-size: 0.9em;
}

.tech-icon {
  width: 24px;
  height: 24px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 8px;
}
</style>
