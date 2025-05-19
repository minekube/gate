<script setup>
import DefaultTheme from 'vitepress/theme';
import { useRouter } from 'vitepress';
import { watch } from 'vue';
import HomeHeroImage from './HomeHeroImage.vue';
import LandingAfter from './LandingAfter.vue';
import MyGlobalButton from './DiscordButton.vue';

const { Layout } = DefaultTheme;

const router = useRouter();

// Only run this on the client. Not during build
if (typeof window !== 'undefined' && window.posthog) {
  watch(
    () => router.route.data.relativePath,
    (path) => {
      posthog.capture('$pageview');
    },
    { immediate: true }
  );
}
</script>

<template>
  <Layout>
    <template #home-hero-image>
      <HomeHeroImage />
    </template>
    <template #home-features-after>
      <LandingAfter />
    </template>
    <template #default>
      <slot />
    </template>
    <template #layout-bottom>
      <MyGlobalButton />
    </template>
  </Layout>
</template>
