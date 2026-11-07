<script setup lang="ts">
import RoundArrowBack, { type IconProps } from '@iconify-vue/ic/round-arrow-back'
import type { DefineSetupFnComponent, PublicProps } from 'vue'

const props = defineProps<{
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  Icon: DefineSetupFnComponent<IconProps, {}, {}, IconProps & {}, PublicProps>
  color?: string
  disabled?: boolean
}>()

const emit = defineEmits<{ click: [] }>()
</script>

<template>
  <button class="back-button" @click="emit('click')" :disabled="props.disabled">
    <props.Icon
      :style="{
        width: '100%',
        height: '100%',
        padding: '15%',
        color: props.color ?? 'var(--color-button-icon)',
      }"
    />
  </button>
</template>

<style lang="css" scoped>
.back-button {
  --button-bg-color: rgba(0, 0, 0, 0);
  --button-bg-color-hover: rgba(0, 0, 0, 0.1);
  --button-bg-color-active: rgba(0, 0, 0, 0.15);
  --button-bg-color-disabled: rgba(0, 0, 0, 0);
  width: calc(2 * var(--font-size-big));
  height: calc(2 * var(--font-size-big));
  margin: 0.2rem;
  padding: 0;
  border-radius: 50vmin;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
