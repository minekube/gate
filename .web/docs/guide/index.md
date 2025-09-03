---
title: "Gate Minecraft Proxy Guide - Introduction & Setup"
description: "Complete guide to Gate Minecraft proxy. Learn how to set up, configure, and deploy Gate as a modern replacement for BungeeCord and Velocity with Bedrock cross-play support."
---
# Introduction

_Gate is a modern cloud-native, open source, fast, batteries-included and secure proxy for Minecraft servers
that focuses on scalability, flexibility, multi-version support, **cross-platform compatibility**, and developer friendliness._

---

<div class="feature-image">
  <img src="/images/server-list.png" alt="Gate server list ping example" />
</div>

<!--@include: ../badges.md -->

## What is Gate?

Gate is a lightweight yet powerful Minecraft proxy that can run anywhere - from your local machine to large-scale cloud deployments:

- 🚀 Run locally as a simple [binary](install/binaries)
- 🐳 Deploy with [Docker](install/docker) containers
- ☸️ Scale infinitely in [Kubernetes](install/kubernetes) clusters

It's designed as a modern replacement for legacy proxies like BungeeCord, while maintaining compatibility to run alongside them. Built entirely in Go and inspired by the Velocity project, Gate brings enterprise-grade performance to Minecraft server networks.

::: tip Why Go?
Gate is written in [Go](https://go.dev/) - a modern, fast, and reliable programming language designed by Google.

Go powers the world's largest platforms and is used by companies like:
Google, Microsoft, Meta, Amazon, Twitter, PayPal, Twitch, Netflix, Dropbox, Uber, Cloudflare, Docker, and many more.
:::

## Quick Start

Ready to jump in? Choose your path:

- 🎮 **Server Owners**: Head to the [Quick Start](quick-start) guide
- 💻 **Developers**: Check out the [Developer Guide](/developers/)

## Why Use a Minecraft Proxy?

<div class="feature-cards">
  <div class="feature-card">
    <div class="card-content">
      <h3>🎮 Seamless Player Experience</h3>
      <ul>
        <li>Move players between servers instantly</li>
        <li>No disconnects during server switches</li>
        <li>Smooth transitions between game modes</li>
        <li>Single point of entry for your network</li>
      </ul>
    </div>
  </div>

  <div class="feature-card">
    <div class="card-content">
      <h3>🔌 Network-Wide Features</h3>
      <ul>
        <li>Cross-server chat systems</li>
        <li>Global command handling</li>
        <li>Network-wide player management</li>
        <li>Unified permission systems</li>
      </ul>
    </div>
  </div>

  <a href="/guide/bedrock" class="feature-card" style="text-decoration: none; color: inherit;">
    <div class="card-content">
      <h3>📱 Cross-Platform Support</h3>
      <ul>
        <li>Java Edition (PC) players</li>
        <li>Bedrock Edition (Mobile, Console, Win)</li>
        <li>Built-in Geyser & Floodgate integration</li>
        <li>Zero backend plugins required</li>
      </ul>
    </div>
  </a>

  <div class="feature-card">
    <div class="card-content">
      <h3>🔍 Advanced Monitoring</h3>
      <ul>
        <li>Real-time packet inspection</li>
        <li>Network traffic analysis</li>
        <li>Performance monitoring</li>
        <li>Security audit capabilities</li>
      </ul>
    </div>
  </div>

  <a href="/guide/why" class="feature-card" style="text-decoration: none; color: inherit;">
    <div class="card-content">
      <h3>Why Gate?</h3>
      <ul>
        <li>Minimal resource footprint (10MB RAM)</li>
        <li>Minecraft 1.7 to latest support</li>
        <li>Built-in Bedrock cross-play</li>
        <li>Modern Go-based architecture</li>
        <li>Clean, documented APIs</li>
      </ul>
    </div>
  </a>
</div>

### How It Works

Gate acts as an intelligent middleware between players and your Minecraft servers:

1. Players connect to Gate like a normal Minecraft server
2. Gate forwards connections to your actual game servers (vanilla, Paper, Spigot, etc.)
3. Players can move between servers while maintaining their connection
4. Gate monitors all network traffic and emits events for:
   - Login/Logout
   - Server Connections
   - Chat Messages
   - Player Kicks
   - [And more!](https://github.com/minekube/gate/blob/master/pkg/edition/java/proxy/events.go)

This architecture enables powerful features like load balancing, server maintenance without disconnects, and network-wide plugins.

<style>
.feature-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
  margin: 24px 0;
}

.feature-card {
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  transition: all 0.3s ease;
}

.feature-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 12px 0 var(--vp-c-divider);
  border-color: var(--vp-c-brand-1);
}

.card-content {
  padding: 20px;
}

.feature-card h3 {
  margin-top: 0;
  margin-bottom: 16px;
  color: var(--vp-c-brand-1);
}

.feature-card ul {
  padding-left: 20px;
  margin-bottom: 0;
}

.feature-card li {
  margin: 8px 0;
  color: var(--vp-c-text-2);
}

.feature-image {
  margin: 2rem 0;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.feature-image img {
  width: 100%;
  display: block;
}
</style>
