<script setup>
import Graph from "./Graph.vue";
import VPImage from "./VPImage.vue";

const velocity = 'https://avatars.githubusercontent.com/u/41710604?s=200&v=4'
const paper = 'https://avatars.githubusercontent.com/u/7608950?s=200&v=4'
const spigot = 'https://avatars.githubusercontent.com/u/4350249?s=48&v=4'

const center = {x: 100, y: 0};
const space = 120; // Increase the space constant here

const nodes = [
  {x: center.x, y: center.y, link: '/guide/', image: '/minekube-logo.png', class: 'w-48'}, // Top center node
  {x: center.x - space, y: 200, link: '/guide/lite', image: velocity}, // First bottom node
  {x: center.x, y: 230, link: '/guide/quick-start#configuring-backend-servers', image: paper, class: 'w-16'}, // Second bottom node
  {x: center.x + space, y: 200, link: '/guide/quick-start#configuring-backend-servers', image: spigot}, // Third bottom node
];

// Define connections between nodes by their index
const connections = [
  [0, 1],
  [0, 2],
  [0, 3],
];

</script>

<template>

  <div class="hidden lg:block">
    <Graph :connections="connections" :nodes="nodes"/>
  </div>

  <div class="block lg:hidden">
    <VPImage class="image-src" image="/minekube-logo.png"/>
  </div>

</template>

<style scoped>

</style>