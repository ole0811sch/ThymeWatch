<script setup lang="ts">
import router, { paths } from '@/router'
import IconButton from './IconButton.vue'
import RoundArrowBack from '@iconify-vue/ic/round-arrow-back'

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
      <IconButton
        :Icon="RoundArrowBack"
        @click="router.push(paths[props.backPath])"
        style="width: 1.5rem; height: 1.5rem"
      />
    </div>
    <div style="font-size: var(--font-size-normal); font-weight: bold">
      {{ props.title }}
    </div>
    <div style="flex: 1"></div>
  </nav>
</template>
