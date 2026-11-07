<script setup lang="ts">
import router, { paths } from '@/router'
import BackButton from './BackButton.vue';

const props = defineProps<{
  title: string
  backPath: keyof typeof paths
}>()
</script>

<template>
  <nav
    style="
      border-bottom-style: solid;
      border-bottom-width: 0.05em;
      background-color: var(--color-secondary);
      display: flex;
      flex-direction: row;
      align-items: center;
    "
  >
    <div style="flex: 1; align-items: center; display: grid">
      <BackButton @click="router.push(paths[props.backPath])"/>
    </div>
    <div style="font-size: var(--font-size-normal); font-weight: bold">
      {{ props.title }}
    </div>
    <div style="flex: 1"></div>
  </nav>
</template>
