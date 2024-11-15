<template>
    <div class="VPDoc px-[32px] py-[48px]">
        <div class="flex flex-col gap-2 relative mx-auto max-w-[948px]">
            <h1 class="text-vp-c-text-1 text-3xl font-semibold mb-4">
                {{ searchMode === 'extensions' ? 'Extensions' : 'Projects using Minekube Libraries' }}
            </h1>
            <p class="text-vp-c-text-3 font-normal text-md mb-4">
                <span v-if="searchMode === 'extensions'">
                    Here you can find useful extensions that can improve your Gate proxy!
                    <br />
                    To add your own extension, simply add the <code class="font-bold mx-1">gate-extension</code> topic to your repository on GitHub.
                </span>
                <span v-else>
                    Here you can find projects that use Minekube libraries on GitHub!
                    <br />
                    To add your own project, simply import any <code class="font-bold mx-1">go.minekube.com</code> library in your go.mod file.
                </span>
            </p>

            <!-- Toggle Button for Search Mode -->
            <div class="mb-4">
                <label class="font-semibold mr-2">Search Mode:</label>
                <button
                    @click="toggleSearchMode"
                    :class="{'bg-vp-c-brand-3 text-white': searchMode === 'extensions', 'bg-vp-c-border text-vp-c-text-1': searchMode === 'go-modules'}"
                    class="rounded-lg px-4 py-2 mr-2 focus:outline-none"
                >
                    {{ searchMode === 'extensions' ? 'Extensions' : 'Minekube Libraries' }}
                </button>
            </div>

            <!-- Search Input -->
            <input
                v-model="searchText"
                class="rounded-lg px-3 py-2 w-[calc(100%-2px)] translate-x-[1px] bg-vp-c-bg focus:ring-vp-c-brand-2 text-vp-c-text-2 transition-colors font-base ring-vp-c-border ring-1"
                placeholder="Search..."
            />

            <!-- Show message when cached data is being used -->
            <div v-if="isCachedData && !loading" class="my-3 text-center text-yellow-600">
                <strong>Warning:</strong> Showing locally cached results. To see updated results, please try again later.
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
                        <span
                            class="group-hover:text-vp-c-brand-2 transition-colors"
                            >View on GitHub</span
                        >
                    </p>
                </a>
            </ul>

            <!-- No Results Message -->
            <p v-else class="my-3">{{ noResultsMessage }}</p>
        </div>
    </div>
</template>

<script>
export default {
    name: "ExtensionsList",
    data() {
        return {
            extensions: [],  // To store extensions data
            goModules: [],    // To store go-modules data
            searchText: "",
            loading: false,
            searchMode: "extensions", // Default mode is 'extensions'
            error: null,         // To store error message
            isCachedData: false,  // Flag to indicate if we're showing cached data
        };
    },
    created() {
        this.fetchData(); // Fetch data for both categories on initial load
        this.updateTitle(); // Set the initial title based on default searchMode
    },
    methods: {
        toggleSearchMode() {
            // Toggle between 'extensions' and 'go-modules'
            this.searchMode = this.searchMode === "extensions" ? "go-modules" : "extensions";
            this.updateTitle(); // Update the title when searchMode changes
        },
        async fetchData() {
            const cacheKey = "extensionsAndGoModulesData";
            this.loading = true;
            this.error = null; // Reset error message before fetching data

            try {
                // Attempt to fetch data from the API
                const [extensionsResponse, goModulesResponse] = await Promise.all([
                    fetch("/api/extensions"),
                    fetch("/api/go-modules")
                ]);

                if (!extensionsResponse.ok || !goModulesResponse.ok) {
                    throw new Error("Error fetching data from API");
                }

                const extensionsData = await extensionsResponse.json();
                const goModulesData = await goModulesResponse.json();

                // Process and sort data
                this.extensions = extensionsData
                    .map(item => ({ ...item, stars: Number(item.stars) }))
                    .sort((a, b) => b.stars - a.stars);

                this.goModules = goModulesData
                    .map(item => ({ ...item, stars: Number(item.stars) }))
                    .sort((a, b) => b.stars - a.stars);

                // Cache the data if API request is successful (only if window is available)
                if (typeof window !== "undefined" && window.localStorage) {
                    const currentTime = new Date().getTime();
                    localStorage.setItem(cacheKey, JSON.stringify({
                        extensions: this.extensions,
                        goModules: this.goModules,
                        timestamp: currentTime,
                    }));
                }

                this.isCachedData = false; // No need to show cached data warning
            } catch (error) {
                console.error("Error fetching data:", error);
                this.error = "Error reaching the API."; // Set error message

                // Check if there is cached data (only if window is available)
                if (typeof window !== "undefined" && window.localStorage) {
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
            const title = this.searchMode === "extensions"
                ? "Extensions | Gate Proxy"
                : "Minekube Libraries | Gate Proxy";
            document.title = title;
        },
    },
    computed: {
        filteredExtensions() {
            const data = this.searchMode === "extensions" ? this.extensions : this.goModules;
            return data.filter((item) =>
                item.name.toLowerCase().includes(this.searchText.toLowerCase())
            );
        },
        noResultsMessage() {
            // If no results are found, show a generic message
            if (this.filteredExtensions.length === 0) {
                const message = this.searchMode === "extensions" 
                    ? "No extensions found"
                    : "No projects found";

                return `${message}. Make sure you're typing the name correctly, or try again later.`;
            }

            return "";
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
