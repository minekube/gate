<script lang="ts" setup>
import type {DefaultTheme} from 'vitepress/theme'
import {withBase} from 'vitepress'

defineProps<{
  image: DefaultTheme.ThemeableImage
  alt?: string
}>()

defineOptions({inheritAttrs: false})
</script>

<template>
  <template v-if="image">
    <img
        v-if="typeof image === 'string' || 'src' in image"
        :alt="alt ?? (typeof image === 'string' ? '' : image.alt || '')"
        :src="withBase(typeof image === 'string' ? image : image.src)"
        class="VPImage"
        v-bind="typeof image === 'string' ? $attrs : { ...image, ...$attrs }"
    />
    <template v-else>
      <VPImage
          :alt="image.alt"
          :image="image.dark"
          class="dark"
          v-bind="$attrs"
      />
      <VPImage
          :alt="image.alt"
          :image="image.light"
          class="light"
          v-bind="$attrs"
      />
    </template>
  </template>
</template>

<style scoped>
html:not(.dark) .VPImage.dark {
  display: none;
}

.dark .VPImage.light {
  display: none;
}
</style>
