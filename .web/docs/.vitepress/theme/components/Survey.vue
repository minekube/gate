<template>
  <template v-if="phid && phsid">
    <iframe
      :src="`https://formless.ai/c/PMr93Z8ztjUe?phid=${phid}&phsid=${phsid}`"
      class="formless-embed"
      width="100%"
      height="100%"
      loading="lazy"
      allow="microphone"
      style="border: 0; display: block; height: 100vh"
    ></iframe>
  </template>
</template>

<script setup>
import { ref, onMounted } from 'vue';

const phid = ref('');
const phsid = ref('');

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
    } else {
      // Retry after a short delay if PostHog isn't loaded yet
      setTimeout(getPostHogIds, 10);
    }
  };

  getPostHogIds();
});

// Load the Formless AI script dynamically
const loadFormlessScript = () => {
  const script = document.createElement('script');
  script.src = 'https://embed.formless.ai/embed.js';
  script.async = true;
  document.body.appendChild(script);
};

loadFormlessScript();
</script>
