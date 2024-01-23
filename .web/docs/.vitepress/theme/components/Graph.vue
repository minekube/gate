<script setup>
import {computed} from 'vue';
import VPImage from "./VPImage.vue";

// Define props
const props = defineProps({
  nodes: Array,
  connections: Array
});

// Compute paths for each connection
const paths = computed(() => {
  return props.connections.map(([i, j]) => {
    const node1 = props.nodes[i];
    const node2 = props.nodes[j];
    const dx = node2.x - node1.x; // x distance between nodes
    const dy = node2.y - node1.y; // y distance between nodes
    const distance = Math.sqrt(dx * dx + dy * dy); // Euclidean distance between nodes
    const curviness = distance * 0.2; // Curviness as a function of distance
    const cx1 = (node1.x + node2.x) / 2 - dx * 0.2; // x-coordinate of first control point
    const cy1 = Math.max(node1.y, node2.y) - curviness; // y-coordinate of first control point
    const cx2 = (node1.x + node2.x) / 2 + dx * 0.2; // x-coordinate of second control point
    const cy2 = cy1; // y-coordinate of second control point
    return `M${node1.x} ${node1.y} C${cx1} ${cy1} ${cx2} ${cy2} ${node2.x} ${node2.y}`;
  });
});

// Compute the maximum x and y coordinates
const maxWidth = computed(() => Math.max(...props.nodes.map(node => node.x), ...props.connections.map(([i, j]) => Math.max(props.nodes[i].x, props.nodes[j].x))));
const maxHeight = computed(() => Math.max(...props.nodes.map(node => node.y), ...props.connections.map(([i, j]) => Math.max(props.nodes[i].y, props.nodes[j].y))));

</script>

<template>
  <div class="animated relative">
    <!-- Create each node -->
    <div v-for="(node, index) in nodes" :key="index"
         :style="{ position: 'absolute', left: `${node.x}px`, top: `${node.y}px`, zIndex: index }">
      <a v-if="node.link" :href="node.link">
        <VPImage v-if="node.image" :alt="`Image ${index + 1}`" :class="`image-src w-20 rounded-2xl ${node.class ?? ''}`"
                 :image="node.image"/>
        <div v-else class="node-html" v-html="node.content"></div>
      </a>
      <template v-else>
        <VPImage v-if="node.image" :alt="`Image ${index + 1}`" :class="`image-src w-20 rounded-2xl ${node.class ?? ''}`"
                 :image="node.image"/>
        <div v-else class="node-html" v-html="node.content"></div>
      </template>
    </div>

    <svg :height="maxHeight" :width="maxWidth">
      <!-- Create paths for each connection -->
      <path v-for="(d, index) in paths" :key="index" :d="d" class="dashed-line" fill="none"/>
    </svg>
  </div>
</template>

<style scoped>
@keyframes animation {
  0% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(20px);
  }
  100% {
    transform: translateY(0);
  }
}

.animated {
  animation: animation 4s infinite ease-in-out;
}

@keyframes dashdraw {
  from {
    stroke-dashoffset: 1000;
  }
}

.dashed-line {
  stroke-width: 2;
  stroke: var(--vp-c-text-1);
  stroke-dasharray: 5;
  stroke-dashoffset: 0;
  animation: dashdraw linear infinite 50s;
}
</style>