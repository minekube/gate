<template>
  <div class="VPDoc px-[32px] py-[48px]">
    <div class="flex flex-col gap-2 relative mx-auto max-w-[948px]">
      <h1 class="text-vp-c-text-1 text-3xl font-semibold mb-4">
        {{
          searchMode === 'extensions'
            ? 'Gate Extensions'
            : 'Gate Libraries/Projects'
        }}
      </h1>
      <p class="text-vp-c-text-3 font-normal text-md mb-4">
        <span v-if="searchMode === 'extensions'">
          Here you can find useful extensions that can improve your Gate proxy!
          <br />
          To add your own extension, simply add the
          <code class="font-bold mx-1">gate-extension</code> topic to your
          repository on GitHub.
        </span>
        <span v-else>
          Here you can find projects that use Minekube libraries on GitHub!
          <br />
          To add your own project, simply import any
          <code class="font-bold mx-1">go.minekube.com</code> library in your
          go.mod file.
        </span>
      </p>

      <!-- Toggle Button for Search Mode -->
      <div class="mb-6">
        <div
          class="inline-flex rounded-lg bg-vp-c-brand dark:bg-vp-c-brand p-1"
        >
          <button
            @click="
              () => {
                searchMode = 'extensions';
              }
            "
            :class="{
              'bg-white dark:bg-gray-900 text-vp-c-brand dark:text-white font-semibold ring-2 ring-white/50 dark:ring-gray-700 ring-offset-2 ring-offset-vp-c-brand':
                searchMode === 'extensions',
              'text-vp-c-text-1 dark:text-gray-300 hover:text-white hover:bg-white/10':
                searchMode !== 'extensions',
            }"
            class="px-6 py-2.5 rounded-md transition-all duration-200 cursor-pointer flex items-center gap-2"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fill-rule="evenodd"
                d="M12.316 3.051a1 1 0 01.633 1.265l-4 12a1 1 0 11-1.898-.632l4-12a1 1 0 011.265-.633zM5.707 6.293a1 1 0 010 1.414L3.414 10l2.293 2.293a1 1 0 11-1.414 1.414l-3-3a1 1 0 010-1.414l3-3a1 1 0 011.414 0zm8.586 0a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 11-1.414-1.414L16.586 10l-2.293-2.293a1 1 0 010-1.414z"
                clip-rule="evenodd"
              />
            </svg>
            Extensions
          </button>
          <button
            @click="
              () => {
                searchMode = 'go-modules';
              }
            "
            :class="{
              'bg-white dark:bg-gray-900 text-vp-c-brand dark:text-white font-semibold ring-2 ring-white/50 dark:ring-gray-700 ring-offset-2 ring-offset-vp-c-brand':
                searchMode === 'go-modules',
              ' hover:text-[var(--vp-c-text-2)] hover:bg-white/10':
                searchMode !== 'go-modules',
            }"
            class="px-6 py-2.5 rounded-md transition-all duration-200 cursor-pointer flex items-center gap-2"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                d="M7 3a1 1 0 000 2h6a1 1 0 100-2H7zM4 7a1 1 0 011-1h10a1 1 0 110 2H5a1 1 0 01-1-1zM2 11a2 2 0 012-2h12a2 2 0 012 2v4a2 2 0 01-2 2H4a2 2 0 01-2-2v-4z"
              />
            </svg>
            Libraries/Projects
          </button>
        </div>
      </div>

      <!-- Search Input -->
      <div class="relative mb-6">
        <input
          v-model="searchText"
          class="rounded-lg px-4 py-3 w-full bg-vp-c-bg focus:ring-2 focus:ring-vp-c-brand-2 text-vp-c-text-1 transition-all duration-200 font-medium ring-1 ring-vp-c-border focus:outline-none"
          :placeholder="
            searchMode === 'extensions'
              ? 'Search extensions...'
              : 'Search libraries and projects...'
          "
        />
      </div>

      <!-- Show message when cached data is being used -->
      <div
        v-if="isCachedData && !loading"
        class="my-3 text-center text-yellow-600"
      >
        <strong>Warning:</strong> Showing locally cached results. To see updated
        results, please try again later.
      </div>

      <!-- Show loading indicator while data is being fetched -->
      <div v-if="loading" class="my-3 text-center">Loading...</div>

      <!-- Show error message -->
      <div v-if="error && !isCachedData" class="my-3 text-center text-red-600">
        Error reaching the API. To see updated results, please try again later.
      </div>

      <!-- Display Results -->
      <ul
        v-else-if="filteredExtensions.length > 0"
        class="grid grid-cols-1 lg:grid-cols-2 gap-2"
      >
        <a
          v-for="item in filteredExtensions"
          :key="item.name"
          :href="item.url"
          class="p-4 group bg-vp-c-bg transition-all flex flex-col rounded-lg border border-vp-c-border hover:border-vp-c-brand-2 animate-in fade-in-40 relative"
        >
          <h2 class="font-bold">
            {{ item.name }}
            <span class="font-normal"> by </span>
            <span>{{ item.owner }}</span>
          </h2>
          <p class="text-vp-c-text-2 mb-2">
            {{ item.description }}
          </p>
          <p class="text-vp-c-text-3 mt-auto flex flex-row">
            <span class="mr-auto">{{ item.stars }} stars</span>
            <span class="group-hover:text-vp-c-brand-2 transition-colors"
              >View on GitHub</span
            >
          </p>
        </a>
      </ul>

      <!-- No Results Message -->
      <p v-else class="my-3 text-center text-vp-c-text-2">
        {{
          searchMode === 'extensions'
            ? 'No extensions found'
            : 'No projects found'
        }}. Make sure you're typing the name correctly, or check out our awesome
        list below!
      </p>

      <!-- New permanent link to awesome repo -->
      <div
        class="mt-8 p-6 bg-vp-c-bg border-2 border-vp-c-border rounded-lg text-center"
      >
        <h3 class="text-xl font-semibold mb-2 text-vp-c-text-1">
          Looking for more?
        </h3>
        <p class="text-vp-c-text-2 mb-4">
          Discover our curated collection of Gate projects and extensions!
        </p>
        <a
          href="https://github.com/minekube/awesome"
          class="inline-flex items-center gap-2 px-6 py-3 bg-[#24292e] text-white rounded-lg hover:bg-[#2f363d] transition-colors font-medium shadow-sm"
        >
          <!-- GitHub Icon -->
          <svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
            <path
              fill-rule="evenodd"
              clip-rule="evenodd"
              d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.87 8.17 6.84 9.5.5.08.66-.23.66-.5v-1.69c-2.77.6-3.36-1.34-3.36-1.34-.46-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.87 1.52 2.34 1.07 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.55-1.11-4.55-4.92 0-1.11.38-2 1.03-2.71-.1-.25-.45-1.29.1-2.64 0 0 .84-.27 2.75 1.02.79-.22 1.65-.33 2.5-.33.85 0 1.71.11 2.5.33 1.91-1.29 2.75-1.02 2.75-1.02.55 1.35.2 2.39.1 2.64.65.71 1.03 1.6 1.03 2.71 0 3.82-2.34 4.66-4.57 4.91.36.31.69.92.69 1.85V21c0 .27.16.59.67.5C19.14 20.16 22 16.42 22 12A10 10 0 0012 2z"
            />
          </svg>
          Browse Awesome List
          <!-- External Link Arrow -->
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="h-5 w-5"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              d="M11 3a1 1 0 100 2h2.586l-6.293 6.293a1 1 0 101.414 1.414L15 6.414V9a1 1 0 102 0V4a1 1 0 00-1-1h-5z"
            />
            <path
              d="M5 5a2 2 0 00-2 2v8a2 2 0 002 2h8a2 2 0 002-2v-3a1 1 0 10-2 0v3H5V7h3a1 1 0 000-2H5z"
            />
          </svg>
        </a>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'ExtensionsList',
  data() {
    return {
      extensions: [], // To store extensions data
      goModules: [], // To store go-modules data
      searchText: '',
      loading: false,
      searchMode: 'extensions', // Default mode is 'extensions'
      error: null, // To store error message
      isCachedData: false, // Flag to indicate if we're showing cached data
    };
  },
  created() {
    this.fetchData(); // Fetch data for both categories on initial load
    this.updateTitle(); // Set the initial title based on default searchMode
  },
  methods: {
    toggleSearchMode() {
      // Toggle between 'extensions' and 'go-modules'
      this.searchMode =
        this.searchMode === 'extensions' ? 'go-modules' : 'extensions';
      this.updateTitle(); // Update the title when searchMode changes
    },
    async fetchData() {
      const cacheKey = 'extensionsAndGoModulesData';
      this.loading = true;
      this.error = null; // Reset error message before fetching data

      try {
        // Attempt to fetch data from the API
        const [extensionsData, goModulesData] = await Promise.all([
          fetch('/api/extensions').then((res) => {
            if (!res.ok) throw new Error('Error fetching extensions data');
            return res.json();
          }),
          fetch('/api/go-modules').then((res) => {
            if (!res.ok) throw new Error('Error fetching go-modules data');
            return res.json();
          }),
        ]);

        // Process and sort data
        this.extensions = extensionsData
          .map((item) => ({ ...item, stars: Number(item.stars) }))
          .sort((a, b) => b.stars - a.stars);

        this.goModules = goModulesData
          .map((item) => ({ ...item, stars: Number(item.stars) }))
          .sort((a, b) => b.stars - a.stars);

        // Cache the data if API request is successful (only if window is available)
        if (typeof window !== 'undefined' && window.localStorage) {
          const currentTime = new Date().getTime();
          localStorage.setItem(
            cacheKey,
            JSON.stringify({
              extensions: this.extensions,
              goModules: this.goModules,
              timestamp: currentTime,
            })
          );
        }

        this.isCachedData = false; // No need to show cached data warning
      } catch (error) {
        console.error('Error fetching data:', error);
        this.error = 'Error reaching the API.'; // Set error message

        // Check if there is cached data (only if window is available)
        if (typeof window !== 'undefined' && window.localStorage) {
          const cachedData = JSON.parse(localStorage.getItem(cacheKey));
          if (cachedData) {
            this.extensions = cachedData.extensions;
            this.goModules = cachedData.goModules;
            this.isCachedData = true; // Indicate cached data is being used
          }
        }
      } finally {
        this.loading = false;
      }
    },
    updateTitle() {
      // Dynamically set the tab title based on the current search mode
      const title =
        this.searchMode === 'extensions'
          ? 'Extensions | Gate Proxy'
          : 'Minekube Libraries | Gate Proxy';
      document.title = title;
    },
  },
  computed: {
    filteredExtensions() {
      const data =
        this.searchMode === 'extensions' ? this.extensions : this.goModules;
      return data.filter((item) =>
        item.name.toLowerCase().includes(this.searchText.toLowerCase())
      );
    },
    noResultsMessage() {
      if (this.filteredExtensions.length === 0) {
        const message =
          this.searchMode === 'extensions'
            ? 'No extensions found'
            : 'No projects found';

        return `${message}. Check out our <a href="https://github.com/minekube/awesome" 
                        class="text-vp-c-brand hover:underline font-medium">awesome list</a> 
                        for more Gate projects and extensions!`;
      }
      return '';
    },
  },
  watch: {
    // Watch for changes in searchMode and update the title accordingly
    searchMode() {
      this.updateTitle();
    },
  },
};
</script>
