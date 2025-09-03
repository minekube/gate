---
title: "Gate TypeScript API - Minecraft Proxy Development"
description: "Build Minecraft proxy extensions with Gate TypeScript API. Modern JavaScript/TypeScript SDK with comprehensive documentation."
---

# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/typescript/typescript-original.svg" class="tech-icon" alt="TypeScript" /> TypeScript Guide

Gate's TypeScript/JavaScript client libraries allow you to interact with Gate's API using your preferred runtime environment. This guide covers installation and usage across different JavaScript runtimes.

## Installation

Choose your preferred runtime environment:

<div class="vp-features">
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="./bun" class="feature-link">
      <div class="title">
        <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/bun/bun-original.svg" class="tech-icon" alt="Bun" />
        Bun
      </div>
      <div class="details">ultra-fast runtime</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="./node" class="feature-link">
      <div class="title">
        <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/nodejs/nodejs-original.svg" class="tech-icon" alt="Node.js" />
        Node.js
      </div>
      <div class="details">with pnpm</div>
    </a>
  </div>
  <div class="vp-feature-small">
    <a style="text-decoration: none" href="./web" class="feature-link">
      <div class="title">
        <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/chrome/chrome-original.svg" class="tech-icon" alt="Web" />
        Web
      </div>
      <div class="details">browser support</div>
    </a>
  </div>
</div>

## Features

- **Type Safety**: Full TypeScript support with generated types
- **Modern APIs**: Promise-based async/await API
- **Cross-Platform**: Works in Node.js, Bun, and web browsers
- **Efficient**: Uses Protocol Buffers for efficient data transfer
- **Secure**: HTTPS support with customizable transport options

## Common Use Cases

- Building admin panels and dashboards
- Creating Discord bots
- Automating server management
- Developing custom monitoring tools
- Integration with existing TypeScript/JavaScript applications

<style>
.vp-features {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 20px;
  margin: 20px 0;
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
  width: 32px;
  height: 32px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 12px;
  position: relative;
  top: -2px;
}

.vp-feature-small .tech-icon {
  width: 24px;
  height: 24px;
  margin-right: 8px;
  top: 0;
}
</style>
