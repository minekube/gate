<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';
import { widgetBotConfig } from '../../widgetbot.config';
import type { CrateInstance } from '../types/widgetbot';

const props = defineProps({
  serverId: {
    type: String,
    default: widgetBotConfig.serverId
  },
  channelId: {
    type: String,
    default: widgetBotConfig.channelId
  }
});

let crate: CrateInstance | null = null;
const isLoaded = ref(false);

onMounted(() => {
  // Only load on client side
  if (typeof window !== 'undefined') {
    // Load WidgetBot Crate script
    const script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/@widgetbot/crate@3';
    script.async = true;
    script.defer = true;
    
    script.onload = () => {
      // Initialize WidgetBot Crate after script loads
      if (window.Crate) {
        try {
          crate = new window.Crate({
            server: props.serverId,
            channel: props.channelId,
            location: widgetBotConfig.position,
            color: widgetBotConfig.color,
            glyph: ['https://cdn.jsdelivr.net/npm/@widgetbot/crate@3/dist/assets/discord.svg', '100%'],
            css: `
              .crate {
                z-index: 40 !important;
              }
            `,
            indicator: true, // Show notification indicator
            notifications: true, // Enable notifications
            timeout: 0, // Keep widget open
          });
          isLoaded.value = true;
        } catch (error) {
          console.error('Failed to initialize WidgetBot:', error);
        }
      }
    };
    
    script.onerror = () => {
      console.error('Failed to load WidgetBot script');
    };
    
    document.head.appendChild(script);
  }
});

onUnmounted(() => {
  // Clean up when component is destroyed
  if (crate && crate.kill) {
    crate.kill();
  }
});
</script>

<template>
  <!-- WidgetBot widget is injected by the script -->
  <div id="widgetbot"></div>
</template>

<style scoped>
/* Additional styling if needed */
#widgetbot {
  position: fixed;
  bottom: 0;
  right: 0;
  z-index: 40;
}
</style>
