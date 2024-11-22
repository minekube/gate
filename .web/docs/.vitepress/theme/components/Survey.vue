<template>
  <iframe
    :src="tallyUrl"
    loading="lazy"
    width="100%"
    height="500"
    frameborder="0"
    marginheight="0"
    marginwidth="0"
    title="Gate"
  ></iframe>
</template>

<script setup>
import { ref, onMounted } from 'vue';

const phid = ref('');
const phsid = ref('');
const tallyUrl = ref('');

// Function to construct the Tally URL with query parameters
const constructTallyUrl = () => {
  const baseUrl = 'https://tally.so/embed/mZGDBA';
  const params = new URLSearchParams({
    dynamicHeight: '1',
    phid: phid.value,
    phsid: phsid.value,
  });
  tallyUrl.value = `${baseUrl}?${params.toString()}`;
};

onMounted(() => {
  // Wait for PostHog to be available
  const getPostHogIds = () => {
    if (
      window.posthog &&
      typeof window.posthog.get_distinct_id === 'function' &&
      typeof window.posthog.get_session_id === 'function'
    ) {
      phid.value = window.posthog.get_distinct_id();
      phsid.value = window.posthog.get_session_id();
      constructTallyUrl(); // Construct the URL once IDs are available
    } else {
      // Retry after a short delay if PostHog isn't loaded yet
      setTimeout(getPostHogIds, 10);
    }
  };

  getPostHogIds();
});

// Include the Tally widget script in the <head> section of your page
const loadTallyScript = () => {
  const script = document.createElement('script');
  script.src = 'https://tally.so/widgets/embed.js';
  script.async = true;
  document.head.appendChild(script);
};

// Load all embeds on the page
loadTallyScript();
</script>
