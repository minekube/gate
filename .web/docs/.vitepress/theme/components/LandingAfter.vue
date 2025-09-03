<template>
  <!-- Add new extensibility section -->
  <div class="py-24 sm:py-32">
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <div class="mx-auto max-w-2xl lg:text-center">
        <h2 class="text-base font-semibold leading-7 text-[--vp-c-brand-2]">
          Powerful Extensibility
        </h2>
        <p
          class="mt-2 text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          Build with your favorite language
        </p>
        <p class="mt-6 text-lg leading-8 text-[--vp-c-text-2]">
          Gate provides official SDKs for multiple languages. Create plugins and
          integrations using modern APIs and tools you love.
        </p>
      </div>

      <!-- Updated Language cards -->
      <div
        class="mt-16 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3 lg:gap-8"
      >
        <a
          v-for="lang in languages"
          :key="lang.name"
          :href="lang.href"
          class="relative group rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
        >
          <div class="flex items-center gap-4">
            <img :src="lang.icon" :alt="lang.name" class="w-10 h-10" />
            <div>
              <h3 class="text-lg font-semibold text-[--vp-c-text-1]">
                {{ lang.name }}
              </h3>
              <p class="text-sm text-[--vp-c-text-2]">{{ lang.description }}</p>
            </div>
          </div>
          <div class="mt-4 flex items-center gap-2 text-[--vp-c-text-2]">
            <!-- Multiple package managers -->
            <template v-if="lang.packageManagers">
              <div class="flex flex-nowrap items-center gap-2">
                <div
                  v-for="pm in lang.packageManagers"
                  :key="pm.name"
                  class="flex items-center gap-1 bg-[--vp-c-bg-soft] px-2 py-1 rounded h-7"
                >
                  <img
                    :src="pm.icon"
                    :alt="pm.name"
                    class="w-4 h-4 object-contain"
                  />
                  <span class="hidden sm:inline text-xs">{{ pm.name }}</span>
                </div>
              </div>
            </template>
            <!-- Single package manager for Go and Rust -->
            <template v-else-if="['Go', 'Rust'].includes(lang.name)">
              <div
                class="flex items-center gap-1 bg-[--vp-c-bg-soft] px-2 py-1 rounded h-7"
              >
                <span class="text-xs">{{ lang.packageManager.name }}</span>
              </div>
            </template>
            <!-- Multiple package managers for JVM languages -->
            <template v-else>
              <div class="flex flex-nowrap items-center gap-2">
                <div
                  v-for="pm in lang.packageManagers"
                  :key="pm.name"
                  class="flex items-center gap-1 bg-[--vp-c-bg-soft] px-2 py-1 rounded h-7"
                >
                  <img
                    :src="pm.icon"
                    :alt="pm.name"
                    class="w-4 h-4 object-contain"
                  />
                  <span class="hidden sm:inline text-xs">{{ pm.name }}</span>
                </div>
              </div>
            </template>
          </div>
        </a>
      </div>

      <!-- Call to action -->
      <div class="mt-16 flex justify-center">
        <a
          href="/developers/api/"
          class="rounded-full px-6 py-3 text-sm font-semibold text-white bg-[--vp-c-brand-2] hover:bg-[--vp-c-brand-1] transition-colors"
        >
          Explore the API Documentation →
        </a>
      </div>
    </div>
  </div>

  <!-- New installation-focused hero section -->
  <div class="py-24 sm:py-32 bg-[--vp-c-bg-soft]">
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <div class="mx-auto max-w-2xl lg:text-center">
        <h2 class="text-base font-semibold leading-7 text-[--vp-c-brand-2]">
          Get Started in Minutes
        </h2>
        <p
          class="mt-2 text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          The next generation Minecraft proxy
        </p>
        <p class="mt-6 text-lg leading-8 text-[--vp-c-text-2]">
          Choose your preferred installation method and start scaling your
          Minecraft network with Gate's powerful features.
        </p>
      </div>

      <!-- Installation methods grid -->
      <div class="mx-auto mt-16 max-w-2xl lg:max-w-4xl">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <!-- Binary Download -->
          <a
            href="/guide/install/binaries"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  class="lucide lucide-package"
                >
                  <path
                    d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"
                  />
                  <path d="M12 22V12" />
                  <path d="m3.3 7 7.703 4.734a2 2 0 0 0 1.994 0L20.7 7" />
                  <path d="m7.5 4.27 9 5.15" />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Binary Download
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Download prebuilt binary for your OS
                </p>
              </div>
            </div>
            <span
              class="absolute top-4 right-4 text-xs font-medium text-[--vp-c-brand-2] bg-[--vp-c-brand-soft] px-2 py-1 rounded-full"
              >Recommended</span
            >
          </a>

          <!-- Docker -->
          <a
            href="/guide/install/docker"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <img
                  src="https://raw.githubusercontent.com/devicons/devicon/master/icons/docker/docker-original.svg"
                  class="h-8 w-8"
                  alt="Docker"
                />
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Docker
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Run with Docker or Docker Compose
                </p>
              </div>
            </div>
          </a>

          <!-- Go Install -->
          <a
            href="/guide/install/go"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <img
                  src="https://raw.githubusercontent.com/devicons/devicon/master/icons/go/go-original.svg"
                  class="h-8 w-8"
                  alt="Go"
                />
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Go Install
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Build from source with Go tooling
                </p>
              </div>
            </div>
            <span
              class="absolute top-4 right-4 text-xs font-medium text-[--vp-c-text-2] bg-[--vp-c-bg-soft] px-2 py-1 rounded-full"
              >Developers</span
            >
          </a>

          <!-- Kubernetes -->
          <a
            href="/guide/install/kubernetes"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <img
                  src="https://raw.githubusercontent.com/devicons/devicon/master/icons/kubernetes/kubernetes-original.svg"
                  class="h-8 w-8"
                  alt="Kubernetes"
                />
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Kubernetes
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Deploy on Kubernetes cluster
                </p>
              </div>
            </div>
            <span
              class="absolute top-4 right-4 text-xs font-medium text-[--vp-c-text-2] bg-[--vp-c-bg-soft] px-2 py-1 rounded-full"
              >Advanced</span
            >
          </a>
        </div>

        <!-- Quick command with OS detection and manual toggle -->
        <div class="mt-8 flex flex-col items-center px-4 sm:px-0">
          <!-- OS Toggle - Centered -->
          <div
            class="mb-4 inline-flex items-center rounded-full bg-[--vp-c-bg-alt] p-1"
          >
            <button
              @click="showWindowsCommand = false"
              :class="[
                'flex items-center gap-2 px-3 py-1.5 text-sm transition-colors rounded-full',
                !showWindowsCommand
                  ? 'text-[--vp-c-text-1] bg-[--vp-c-brand-1]'
                  : 'text-[--vp-c-text-2] hover:text-[--vp-c-text-1]',
              ]"
            >
              <div class="flex items-center gap-1">
                <img
                  src="https://raw.githubusercontent.com/devicons/devicon/master/icons/linux/linux-original.svg"
                  class="h-4 w-4"
                  alt="Linux"
                />
                <img
                  src="https://raw.githubusercontent.com/devicons/devicon/master/icons/apple/apple-original.svg"
                  class="h-4 w-4"
                  alt="macOS"
                />
              </div>
              <span>Unix</span>
            </button>
            <button
              @click="showWindowsCommand = true"
              :class="[
                'flex items-center gap-2 px-3 py-1.5 text-sm transition-colors rounded-full',
                showWindowsCommand
                  ? 'text-[--vp-c-text-1] bg-[--vp-c-brand-1]'
                  : 'text-[--vp-c-text-2] hover:text-[--vp-c-text-1]',
              ]"
            >
              <img
                src="https://raw.githubusercontent.com/devicons/devicon/master/icons/windows8/windows8-original.svg"
                class="h-4 w-4"
                alt="Windows"
              />
              <span>Windows</span>
            </button>
          </div>

          <!-- Command Box -->
          <div class="relative group w-full max-w-full sm:max-w-2xl">
            <div
              class="absolute -inset-0.5 bg-gradient-to-r from-[--vp-c-brand-2] to-[--vp-c-brand-1] rounded-xl blur opacity-30 group-hover:opacity-100 transition duration-1000 group-hover:duration-200"
            ></div>
            <div
              class="relative flex items-center bg-[--vp-c-bg-alt] px-4 py-4 rounded-xl"
            >
              <div class="flex-1 overflow-x-auto scrollbar-hide">
                <div class="min-w-0 flex items-center">
                  <code
                    class="text-[13px] font-mono text-[--vp-c-text-1] truncate"
                  >
                    {{
                      showWindowsCommand
                        ? 'powershell -c "irm https://gate.minekube.com/install.ps1 | iex"'
                        : 'curl -sSL https://gate.minekube.com/install.sh | sh'
                    }}
                  </code>
                </div>
              </div>
              <button
                class="ml-4 flex-shrink-0 text-[--vp-c-brand-2] hover:text-[--vp-c-brand-1] transition-colors"
                @click="copyInstallCommand"
              >
                <svg
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M15.666 3.888A2.25 2.25 0 0 0 13.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 0 1-.75.75H9a.75.75 0 0 1-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 0 1-2.25 2.25H6.75A2.25 2.25 0 0 1 4.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 0 1 1.927-.184"
                  />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Bedrock Edition Support Section -->
  <div
    class="py-16 sm:py-20 bg-gradient-to-b from-[--vp-c-bg] to-[--vp-c-bg-soft]"
  >
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <div class="mx-auto max-w-2xl lg:text-center">
        <h2 class="text-base font-semibold leading-7 text-[--vp-c-brand-2]">
          Cross-Play Ready
        </h2>
        <p
          class="mt-2 text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          Bedrock Edition Support
        </p>
        <p class="mt-6 text-lg leading-8 text-[--vp-c-text-2]">
          Unite Java and Bedrock players on the same server with built-in Geyser
          integration. Use familiar tooling and plugins while welcoming players
          from every platform.
        </p>
        <!-- Feature highlights -->
        <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
          <div
            class="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-gradient-to-r from-blue-50 to-blue-100 dark:from-blue-950/50 dark:to-blue-900/50 border border-blue-200 dark:border-blue-800"
          >
            <div class="w-2 h-2 rounded-full bg-blue-500"></div>
            <span class="text-sm font-medium text-blue-700 dark:text-blue-300"
              >Cross-Play via Geyser</span
            >
          </div>
          <div
            class="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-gradient-to-r from-green-50 to-green-100 dark:from-green-950/50 dark:to-green-900/50 border border-green-200 dark:border-green-800"
          >
            <div class="w-2 h-2 rounded-full bg-green-500"></div>
            <span class="text-sm font-medium text-green-700 dark:text-green-300"
              >Managed Mode Available</span
            >
          </div>
          <div
            class="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-gradient-to-r from-purple-50 to-purple-100 dark:from-purple-950/50 dark:to-purple-900/50 border border-purple-200 dark:border-purple-800"
          >
            <div class="w-2 h-2 rounded-full bg-purple-500"></div>
            <span
              class="text-sm font-medium text-purple-700 dark:text-purple-300"
              >No Backend Plugins</span
            >
          </div>
        </div>
      </div>

      <!-- Compact feature summary only (keep section within one viewport) -->

      <!-- Architecture flow (enhanced design) -->
      <div class="mt-8 flex justify-center">
        <div class="w-full max-w-4xl">
          <div class="relative">
            <!-- Background glow -->
            <div
              class="absolute inset-0 bg-gradient-to-r from-[--vp-c-brand-2]/10 via-[--vp-c-brand-1]/10 to-[--vp-c-brand-2]/10 rounded-2xl blur-xl"
            ></div>

            <!-- Main flow -->
            <div
              class="relative flex items-center justify-center gap-6 sm:gap-8 p-6 sm:p-8 bg-[--vp-c-bg-alt]/80 backdrop-blur-sm rounded-2xl border border-[--vp-c-divider]"
            >
              <!-- Geyser -->
              <div class="flex flex-col items-center gap-2">
                <div class="relative">
                  <div
                    class="absolute -inset-1 bg-gradient-to-r from-blue-500 to-blue-600 rounded-xl blur opacity-25"
                  ></div>
                  <img
                    src="https://github.com/GeyserMC.png"
                    alt="Geyser"
                    class="relative h-12 w-12 rounded-xl border-2 border-blue-200 dark:border-blue-800 bg-white dark:bg-gray-900"
                  />
                </div>
                <span
                  class="text-xs font-medium text-blue-600 dark:text-blue-400"
                  >Geyser</span
                >
              </div>

              <!-- Arrow 1 -->
              <div class="flex items-center">
                <svg
                  class="h-5 w-5 text-[--vp-c-brand-1] animate-pulse"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 5l7 7-7 7"
                  />
                </svg>
              </div>

              <!-- Gate (larger, central) -->
              <div class="flex flex-col items-center gap-2">
                <div class="relative">
                  <div
                    class="absolute -inset-1 bg-gradient-to-r from-[--vp-c-brand-2] to-[--vp-c-brand-1] rounded-xl blur opacity-30"
                  ></div>
                  <img
                    src="/minekube-logo.png"
                    alt="Gate"
                    class="relative h-14 w-14 rounded-xl border-2 border-[--vp-c-brand-1] bg-white dark:bg-gray-900 p-1"
                  />
                </div>
                <span class="text-xs font-semibold text-[--vp-c-brand-1]"
                  >Gate</span
                >
              </div>

              <!-- Arrow 2 -->
              <div class="flex items-center">
                <svg
                  class="h-5 w-5 text-[--vp-c-brand-1] animate-pulse"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 5l7 7-7 7"
                  />
                </svg>
              </div>

              <!-- Backend -->
              <div class="flex flex-col items-center gap-2">
                <div class="relative">
                  <div
                    class="absolute -inset-1 bg-gradient-to-r from-orange-500 to-orange-600 rounded-xl blur opacity-25"
                  ></div>
                  <img
                    src="https://github.com/PaperMC.png"
                    alt="Backend"
                    class="relative h-12 w-12 rounded-xl border-2 border-orange-200 dark:border-orange-800 bg-white dark:bg-gray-900"
                  />
                </div>
                <span
                  class="text-xs font-medium text-orange-600 dark:text-orange-400"
                  >Backend</span
                >
              </div>
            </div>
          </div>

          <p
            class="mt-4 text-center text-sm text-[--vp-c-text-2] max-w-2xl mx-auto"
          >
            Bedrock players connect via Geyser → Gate translates protocols →
            Backend servers handle gameplay
          </p>
        </div>
      </div>

      <!-- Quick actions: Quick Start + Learn More -->
      <div class="mx-auto mt-6 max-w-2xl lg:max-w-3xl">
        <div class="relative group">
          <div
            class="absolute -inset-0.5 bg-gradient-to-r from-[--vp-c-brand-2] to-[--vp-c-brand-1] rounded-xl blur opacity-30 group-hover:opacity-100 transition duration-1000 group-hover:duration-200"
          ></div>
          <div class="relative bg-[--vp-c-bg-alt] rounded-xl p-4">
            <div class="flex items-center justify-between mb-2">
              <span class="text-sm text-[--vp-c-text-2]">Quick Start</span>
              <button
                @click="copyBedrockCommand"
                class="text-[--vp-c-brand-2] hover:text-[--vp-c-brand-1] transition-all active:scale-90"
                :class="{ 'copy-success': showBedrockCopySuccess }"
              >
                <svg
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    v-if="!showBedrockCopySuccess"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                  />
                  <path
                    v-else
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </button>
            </div>
            <pre
              class="text-sm font-mono overflow-x-auto"
            ><code>git clone https://github.com/minekube/gate.git
cd gate/.examples/bedrock
./start.sh</code></pre>
          </div>
        </div>

        <div class="mt-6 flex items-center justify-center gap-3">
          <a
            href="/guide/bedrock/#-quick-start-30-seconds"
            class="rounded-full px-5 py-2.5 text-sm font-semibold text-white bg-[--vp-c-brand-2] hover:bg-[--vp-c-brand-1] transition-colors"
            >Quick Start</a
          >
          <a
            href="/guide/bedrock/#-configuration-guide"
            class="rounded-full px-5 py-2.5 text-sm font-semibold border border-[--vp-c-divider] text-[--vp-c-text-1] hover:border-[--vp-c-brand-1] transition-colors"
            >Configuration</a
          >
        </div>
      </div>
    </div>
  </div>

  <!-- Lite Mode Section -->
  <div class="py-24 sm:py-32">
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <div class="mx-auto max-w-2xl lg:text-center">
        <h2 class="text-base font-semibold leading-7 text-[--vp-c-brand-2]">
          Ultra-Lightweight Mode
        </h2>
        <p
          class="mt-2 text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          Host-based routing made simple
        </p>
        <p class="mt-6 text-lg leading-8 text-[--vp-c-text-2]">
          Gate Lite acts as an ultra-thin reverse proxy, efficiently routing
          connections based on hostnames. Protect multiple backend servers
          behind a single port with minimal overhead.
        </p>
      </div>

      <!-- Diagram -->
      <div class="mt-16 flex justify-center">
        <div class="w-full max-w-2xl">
          <img
            src="/images/lite-mermaid-diagram-LR.svg"
            alt="Lite Mode Diagram"
            class="w-full h-auto dark:invert dark:opacity-90 object-contain max-h-[200px] opacity-90"
            style="filter: contrast(90%) saturate(95%)"
          />
        </div>
      </div>

      <!-- Feature Grid -->
      <div class="mx-auto mt-16 max-w-2xl lg:max-w-4xl">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <!-- Host Routing -->
          <a
            href="/guide/lite/#host-based-routing"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 104 0 2 2 0 012-2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Host Routing
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">Route by domain</p>
              </div>
            </div>
          </a>

          <!-- Response Caching -->
          <a
            href="/guide/lite/#ping-response-caching"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Response Caching
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Optimized ping responses
                </p>
              </div>
            </div>
          </a>

          <!-- Proxy Support -->
          <a
            href="/guide/lite/#proxy-behind-proxy"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Proxy Support
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Chain multiple proxies
                </p>
              </div>
            </div>
          </a>
        </div>

        <!-- Example Config -->
        <div class="mt-8">
          <div class="relative group">
            <div
              class="absolute -inset-0.5 bg-gradient-to-r from-[--vp-c-brand-2] to-[--vp-c-brand-1] rounded-xl blur opacity-30 group-hover:opacity-100 transition duration-1000 group-hover:duration-200"
            ></div>
            <div class="relative bg-[--vp-c-bg-alt] rounded-xl p-4">
              <div class="flex items-center justify-between mb-2">
                <span class="text-sm text-[--vp-c-text-2]">config.yml</span>
                <button
                  @click="copyLiteConfig"
                  class="text-[--vp-c-brand-2] hover:text-[--vp-c-brand-1] transition-all active:scale-90"
                  :class="{ 'copy-success': showLiteCopySuccess }"
                >
                  <svg
                    class="h-5 w-5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      v-if="!showLiteCopySuccess"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                    <path
                      v-else
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                </button>
              </div>
              <pre class="text-sm font-mono overflow-x-auto"><code>config:
  lite:
    enabled: true
    routes:
      - host: '*.example.com'
        backend: 10.0.0.1:25567
      - host: [ example.com, localhost ]
        backend: [ 10.0.0.2:25566 ]
        cachePingTTL: 3m</code></pre>
            </div>
          </div>
        </div>

        <!-- Learn More Button -->
        <div class="mt-8 flex justify-center">
          <a
            href="/guide/lite/"
            class="rounded-full px-6 py-3 text-sm font-semibold text-white bg-[--vp-c-brand-2] hover:bg-[--vp-c-brand-1] transition-colors"
          >
            Learn More About Lite Mode →
          </a>
        </div>
      </div>
    </div>
  </div>

  <!-- Configuration section -->
  <div class="py-24 sm:py-32 bg-[--vp-c-bg-soft]">
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <div class="mx-auto max-w-2xl lg:text-center">
        <h2 class="text-base font-semibold leading-7 text-[--vp-c-brand-2]">
          Live Configuration
        </h2>
        <p
          class="mt-2 text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          Configure without restarts
        </p>
        <p class="mt-6 text-lg leading-8 text-[--vp-c-text-2]">
          Gate watches your config file for changes and applies them instantly
          without disconnecting players. Switch modes, add servers, or update
          settings - all while staying live.
        </p>
      </div>

      <div class="mx-auto mt-16 max-w-2xl lg:max-w-4xl">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <!-- Live Reload -->
          <a
            href="/guide/config/reload"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Auto Reload
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Changes apply instantly without restarts
                </p>
              </div>
            </div>
          </a>

          <!-- Safe Updates -->
          <a
            href="/guide/config/reload#how-it-works"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Safe Validation
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Changes validated before applying
                </p>
              </div>
            </div>
          </a>

          <!-- Flexible Modes -->
          <a
            href="/guide/lite/"
            class="group relative rounded-2xl border border-[--vp-c-divider] p-6 hover:border-[--vp-c-brand-1] transition-all duration-300 hover:-translate-y-1 hover:shadow-lg bg-[--vp-c-bg-alt]"
          >
            <div class="flex items-center gap-4">
              <div class="flex h-11 w-11 items-center justify-center">
                <svg
                  class="h-6 w-6 text-[--vp-c-brand-1]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5"
                  />
                </svg>
              </div>
              <div>
                <h3
                  class="font-semibold text-[--vp-c-text-1] group-hover:text-[--vp-c-brand-1]"
                >
                  Switch Modes
                </h3>
                <p class="mt-1 text-sm text-[--vp-c-text-2]">
                  Toggle between Lite and Connect modes
                </p>
              </div>
            </div>
          </a>
        </div>

        <!-- Config Example -->
        <div class="mt-8">
          <div class="relative group">
            <div
              class="absolute -inset-0.5 bg-gradient-to-r from-[--vp-c-brand-2] to-[--vp-c-brand-1] rounded-xl blur opacity-30 group-hover:opacity-100 transition duration-1000 group-hover:duration-200"
            ></div>
            <div class="relative bg-[--vp-c-bg-alt] rounded-xl p-4">
              <div class="flex items-center justify-between mb-2">
                <span class="text-sm text-[--vp-c-text-2]">config.yml</span>
                <button
                  @click="copyConfigWithEffect"
                  class="text-[--vp-c-brand-2] hover:text-[--vp-c-brand-1] transition-all active:scale-90"
                  :class="{ 'copy-success': showCopySuccess }"
                >
                  <svg
                    class="h-5 w-5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      v-if="!showCopySuccess"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                    />
                    <path
                      v-else
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                </button>
              </div>
              <pre
                class="text-sm font-mono overflow-x-auto"
              ><code>bind: 0.0.0.0:25565
servers:
  lobby:
    addr: 127.0.0.1:25566
  minigames:
    addr: 127.0.0.1:25567
try:
  - lobby
  - minigames</code></pre>
            </div>
          </div>
        </div>

        <!-- Learn More Button -->
        <div class="mt-8 flex justify-center">
          <a
            href="/guide/config/"
            class="rounded-full px-6 py-3 text-sm font-semibold text-white bg-[--vp-c-brand-2] hover:bg-[--vp-c-brand-1] transition-colors"
          >
            Learn More About Configuration →
          </a>
        </div>
      </div>
    </div>
  </div>

  <!-- Final Call to Action section -->
  <div class="py-16 sm:py-24">
    <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
      <div class="mx-auto max-w-2xl text-center">
        <h2
          class="text-2xl sm:text-3xl font-bold tracking-tight text-[--vp-c-text-1] sm:text-4xl"
        >
          Scale your Minecraft network today
        </h2>
        <p
          class="mt-4 sm:mt-6 text-base sm:text-lg leading-7 sm:leading-8 text-[--vp-c-text-2]"
        >
          Start using Gate in under 5 minutes. Join hundreds of server owners
          building better networks with Minekube technology.
        </p>

        <!-- Social proof stats - made more compact on mobile -->
        <div
          class="mt-6 sm:mt-8 flex justify-center gap-4 sm:gap-8 text-center"
        >
          <div>
            <div
              class="text-2xl sm:text-3xl font-semibold text-[--vp-c-brand-1]"
            >
              800+
            </div>
            <div class="text-xs sm:text-sm text-[--vp-c-text-2]">
              Discord Members
            </div>
          </div>
          <div class="w-px bg-[--vp-c-divider]"></div>
          <div>
            <div
              class="text-2xl sm:text-3xl font-semibold text-[--vp-c-brand-1]"
            >
              750+
            </div>
            <div class="text-xs sm:text-sm text-[--vp-c-text-2]">
              GitHub Stars
            </div>
          </div>
        </div>

        <!-- Buttons stack on mobile -->
        <div
          class="mt-8 sm:mt-10 flex flex-col sm:flex-row items-center justify-center gap-4 sm:gap-6"
        >
          <a
            href="/guide/quick-start"
            class="w-full sm:w-auto rounded-full px-6 py-3 text-sm font-semibold text-white bg-[--vp-c-brand-2] hover:bg-[--vp-c-brand-1] transition-colors"
          >
            Quick Start Guide
          </a>
          <a
            href="https://minekube.com/discord"
            target="_blank"
            rel="noopener"
            class="w-full sm:w-auto flex items-center justify-center gap-2 rounded-full px-6 py-3 text-sm font-semibold text-[--vp-c-text-1] border border-[--vp-c-divider] hover:border-[--vp-c-brand-1] transition-colors bg-[--vp-c-bg-alt]"
          >
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
              <path
                d="M20.317 4.37a19.791 19.791 0 0 0-4.885-1.515a.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0a12.64 12.64 0 0 0-.617-1.25a.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057a19.9 19.9 0 0 0 5.993 3.03a.078.078 0 0 0 .084-.028a14.09 14.09 0 0 0 1.226-1.994a.076.076 0 0 0-.041-.106a13.107 13.107 0 0 1-1.872-.892a.077.077 0 0 1-.008-.128a10.2 10.2 0 0 0 .372-.292a.074.074 0 0 1 .077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127a12.299 12.299 0 0 1-1.873.892a.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028a19.839 19.839 0 0 0 6.002-3.03a.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419c0-1.333.956-2.419 2.157-2.419c1.21 0 2.176 1.096 2.157 2.42c0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419c0-1.333.955-2.419 2.157-2.419c1.21 0 2.176 1.096 2.157 2.42c0 1.333-.946 2.418-2.157 2.418z"
              />
            </svg>
            Join Discord
          </a>
        </div>

        <!-- Social links stack better on mobile -->
        <div
          class="mt-6 sm:mt-8 flex flex-wrap justify-center items-center gap-3 sm:gap-4 text-xs sm:text-sm text-[--vp-c-text-2]"
        >
          <a
            href="https://github.com/minekube/gate"
            target="_blank"
            rel="noopener"
            class="flex items-center gap-2 hover:text-[--vp-c-brand-1] transition-colors"
          >
            <svg
              class="h-4 sm:h-5 w-4 sm:w-5"
              viewBox="0 0 24 24"
              fill="currentColor"
            >
              <path
                d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385c.6.105.825-.255.825-.57c0-.285-.015-1.23-.015-2.235c-3.015.555-3.795-.735-4.035-1.41c-.135-.345-.72-1.41-1.23-1.695c-.42-.225-1.02-.78-.015-.795c.945-.015 1.62.87 1.845 1.23c1.08 1.815 2.805 1.305 3.495.99c.105-.78.42-1.305.765-1.605c-2.67-.3-5.46-1.335-5.46-5.925c0-1.305.465-2.385 1.23-3.225c-.12-.3-.54-1.53.12-3.18c0 0 1.005-.315 3.3 1.23c.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23c.66 1.65.24 2.88.12 3.18c.765.84 1.23 1.905 1.23 3.225c0 4.605-2.805 5.625-5.475 5.925c.435.375.81 1.095.81 2.22c0 1.605-.015 2.895-.015 3.3c0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"
              />
            </svg>
            Star on GitHub
          </a>
          <span class="text-[--vp-c-text-3]">•</span>
          <a
            href="https://twitter.com/minekube"
            target="_blank"
            rel="noopener"
            class="flex items-center gap-2 hover:text-[--vp-c-brand-1] transition-colors"
          >
            <svg
              class="h-4 sm:h-5 w-4 sm:w-5"
              viewBox="0 0 24 24"
              fill="currentColor"
            >
              <path
                d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"
              />
            </svg>
            Follow on X
          </a>
        </div>
      </div>
    </div>
  </div>

  <!-- Survey Section -->
  <div class="py-24 sm:py-32">
    <div class="mx-auto max-w-7xl px-6 lg:px-8">
      <Survey />
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import Survey from './Survey.vue';

const isWindows =
  typeof navigator !== 'undefined' && /Win/.test(navigator.platform);
const showWindowsCommand = ref(isWindows);

// Update copy command function
const copyInstallCommand = () => {
  const command = showWindowsCommand.value
    ? 'powershell -c "irm https://gate.minekube.com/install.ps1 | iex"'
    : 'curl -sSL https://gate.minekube.com/install.sh | sh';
  navigator.clipboard.writeText(command);
};

const showCopySuccess = ref(false);

const copyConfigWithEffect = () => {
  const config = `bind: 0.0.0.0:25565
servers:
  lobby:
    addr: 127.0.0.1:25566
  minigames:
    addr: 127.0.0.1:25567
try:
  - lobby
  - minigames`;

  navigator.clipboard.writeText(config);
  showCopySuccess.value = true;

  setTimeout(() => {
    showCopySuccess.value = false;
  }, 2000);
};

const showLiteCopySuccess = ref(false);

const copyLiteConfig = () => {
  const config = `config:
  lite:
    enabled: true
    routes:
      - host: '*.example.com'
        backend: 10.0.0.1:25567
      - host: [ example.com, localhost ]
        backend: [ 10.0.0.2:25566 ]
        cachePingTTL: 3m`;

  navigator.clipboard.writeText(config);
  showLiteCopySuccess.value = true;

  setTimeout(() => {
    showLiteCopySuccess.value = false;
  }, 2000);
};

const showBedrockCopySuccess = ref(false);

const copyBedrockCommand = () => {
  const command = `git clone https://github.com/minekube/gate.git
cd gate/.examples/bedrock
./start.sh`;

  navigator.clipboard.writeText(command);
  showBedrockCopySuccess.value = true;

  setTimeout(() => {
    showBedrockCopySuccess.value = false;
  }, 2000);
};

const languages = [
  {
    name: 'TypeScript',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/typescript/typescript-original.svg',
    description: 'Build web interfaces and applications',
    href: '/developers/api/typescript/',
    packageManagers: [
      {
        name: 'bun',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/bun/bun-original.svg',
      },
      {
        name: 'pnpm',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/pnpm/pnpm-original.svg',
      },
      {
        name: 'npm',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/npm/npm-original-wordmark.svg',
      },
      {
        name: 'yarn',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/yarn/yarn-original.svg',
      },
    ],
  },
  {
    name: 'Python',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/python/python-original.svg',
    description: 'Create automation scripts and tools',
    href: '/developers/api/python/',
    packageManagers: [
      {
        name: 'uv',
        icon: 'https://docs.astral.sh/uv/assets/logo-letter.svg',
      },
      {
        name: 'pip',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/python/python-original.svg',
      },
      {
        name: 'poetry',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/poetry/poetry-original.svg',
      },
    ],
  },
  {
    name: 'Go',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/go/go-original.svg',
    description: 'Native plugins, maximum performance',
    href: '/developers/api/go/',
    packageManager: {
      name: 'go modules',
    },
  },
  {
    name: 'Rust',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/rust/rust-original.svg',
    description: 'Build high-performance integrations',
    href: '/developers/api/rust/',
    packageManager: {
      name: 'crates',
    },
  },
  {
    name: 'Kotlin',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/kotlin/kotlin-original.svg',
    description: 'Modern JVM development',
    href: '/developers/api/kotlin/',
    packageManagers: [
      {
        name: 'Maven',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/maven/maven-original.svg',
      },
      {
        name: 'Gradle',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/gradle/gradle-original.svg',
      },
    ],
  },
  {
    name: 'Java',
    icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/java/java-original.svg',
    description: 'Enterprise-grade integrations',
    href: '/developers/api/java/',
    packageManagers: [
      {
        name: 'Maven',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/maven/maven-original.svg',
      },
      {
        name: 'Gradle',
        icon: 'https://raw.githubusercontent.com/devicons/devicon/master/icons/gradle/gradle-original.svg',
      },
    ],
  },
];
</script>

<style scoped>
/* Add smooth transition for card hover effects */
.group {
  transition: all 0.3s ease;
}

/* Optional: Add a subtle gradient background to cards on hover */
.group:hover {
  background: linear-gradient(
    to bottom right,
    var(--vp-c-bg-alt),
    var(--vp-c-bg-soft)
  );
}

/* Ensure code blocks maintain readability */
code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas,
    'Liberation Mono', 'Courier New', monospace;
}

.copy-success {
  color: var(--vp-c-green-1);
}

/* Add transition for the copy icon */
.copy-success svg {
  transform: scale(1.1);
}

button svg {
  transition: all 0.2s ease;
}

/* Ensure content is properly spaced */
.mx-auto {
  height: fit-content;
}

/* Improve diagram appearance */
.dark img.dark\:invert {
  filter: invert(1) contrast(85%) saturate(90%) hue-rotate(180deg);
}

img.object-contain {
  object-fit: contain;
  width: 100%;
  margin: 0 auto;
  transition: opacity 0.3s ease;
}

/* Optional hover effect */
img.object-contain:hover {
  opacity: 1;
}

/* Hide scrollbar but keep functionality */
.scrollbar-hide {
  -ms-overflow-style: none; /* IE and Edge */
  scrollbar-width: none; /* Firefox */
}
.scrollbar-hide::-webkit-scrollbar {
  display: none; /* Chrome, Safari and Opera */
}

/* Ensure the command text doesn't wrap but scrolls horizontally */
.whitespace-nowrap {
  white-space: nowrap;
}

/* Improve command box text handling */
.truncate {
  text-overflow: ellipsis;
}

/* Hide scrollbar but keep functionality */
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
</style>
