<script setup lang="ts">
import RoundArrowBack from '@iconify-vue/ic/round-arrow-back'

const emit = defineEmits<{ click: [] }>()
</script>

<template>
  <button class="back-button" @click="emit('click')">
    <RoundArrowBack style="width: 100%; height: 100%" />
  </button>
</template>

<style lang="css" scoped>
.back-button {
  --button-bg-color: rgba(0, 0, 0, 0);
  --button-bg-color-hover: rgba(0, 0, 0, 0.1);
  --button-bg-color-active: rgba(0, 0, 0, 0.15);
  --button-bg-color-disabled: rgba(0, 0, 0, 0);
  width: 1.5rem;
  height: 1.5rem;
  padding: 0rem;
  margin: 0.2rem;
  border-radius: 50%;
}


</style>
