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

            <div v-if="loading" class="my-3 text-center">Loading...</div>
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
            <p v-else class="my-3">No extensions found.</p>
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
        };
    },
    created() {
        this.fetchData(); // Fetch data for both categories on initial load
    },
    methods: {
        toggleSearchMode() {
            // Toggle between 'extensions' and 'go-modules'
            this.searchMode = this.searchMode === "extensions" ? "go-modules" : "extensions";
        },
        async fetchData() {
            const cacheKey = "extensionsAndGoModulesData";
            const cacheExpiration = 60 * 60 * 1000; // Cache expiration time (1 hour in ms)
            const currentTime = new Date().getTime();

            // Check if cached data exists and is still valid (less than an hour old)
            const cachedData = JSON.parse(localStorage.getItem(cacheKey));
            if (cachedData && (currentTime - cachedData.timestamp) < cacheExpiration) {
                this.extensions = cachedData.extensions;
                this.goModules = cachedData.goModules;
                return;
            }

            this.loading = true;
            try {
                // Fetch both extensions and go-modules in parallel
                const [extensionsResponse, goModulesResponse] = await Promise.all([
                    fetch("/api/extensions"),
                    fetch("/api/go-modules")
                ]);

                const extensionsData = await extensionsResponse.json();
                const goModulesData = await goModulesResponse.json();

                if (!Array.isArray(extensionsData) || !Array.isArray(goModulesData)) {
                    console.error("Malformed response from server when requesting extensions or go-modules");
                    return;
                }

                // Ensure stars are treated as numbers and sort by stars (in descending order)
                this.extensions = extensionsData
                    .map(item => ({ ...item, stars: Number(item.stars) }))  // Ensure stars is a number
                    .sort((a, b) => b.stars - a.stars);

                this.goModules = goModulesData
                    .map(item => ({ ...item, stars: Number(item.stars) }))  // Ensure stars is a number
                    .sort((a, b) => b.stars - a.stars);

                // Cache the data with a timestamp
                localStorage.setItem(cacheKey, JSON.stringify({
                    extensions: this.extensions,
                    goModules: this.goModules,
                    timestamp: currentTime
                }));
            } catch (error) {
                console.error("Error fetching data:", error);
                this.extensions = [];
                this.goModules = [];
            } finally {
                this.loading = false;
            }
        },
    },
    computed: {
        filteredExtensions() {
            const data = this.searchMode === "extensions" ? this.extensions : this.goModules;
            return data.filter((item) =>
                item.name.toLowerCase().includes(this.searchText.toLowerCase())
            );
        },
    },
};
</script>
