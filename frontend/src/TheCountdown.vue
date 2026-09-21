<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Countdown } from './CountdownList.vue'

const props = defineProps<{ countdown: Countdown }>()
const emit = defineEmits<{ requestRemove: []; expired: [Countdown] }>()

const displayText = ref('')

function update() {
  if (!props.countdown.expiryTimestamp) {
    displayText.value = ''
    return
  }
  const timeLeft = Math.max(props.countdown.expiryTimestamp.getTime() - Date.now(), 0)
  const secondsLeft = Math.round(timeLeft / 1000)
  const minutesLeft = Math.floor(secondsLeft / 60)
  const hoursLeft = Math.floor(minutesLeft / 60)
  const secondsField = secondsLeft % 60
  const minutesField = minutesLeft % 60
  displayText.value = `${String(hoursLeft).padStart(2, '0')}:${String(minutesField).padStart(2, '0')}:${String(secondsField).padStart(2, '0')}`
}

const onExpired = () => {
  unwatchExpiryTimestamp()
  emit('expired', props.countdown)
}
let callbackId: null | number = null
const unwatchExpiryTimestamp = watch(
  () => props.countdown.expiryTimestamp,
  (value) => {
    if (callbackId !== null) {
      clearTimeout(callbackId)
      callbackId = null
    }
    if (value !== null) {
      callbackId = setTimeout(onExpired, value.getTime() - Date.now())
    }
  },
  { immediate: true },
)
update()

defineExpose({ update })
</script>
<template>
  <div style="display: flex; align-items: start">
    <span>{{ displayText }}</span>
    <button @click="$emit('requestRemove')">Remove</button>
    <details :style="{ display: 'inline' }">
      <summary>Show details</summary>
      <div v-if="props.countdown.name">Name: {{ props.countdown.name }}</div>
      <div>Started at {{ props.countdown.startedAt.toLocaleTimeString() }}</div>
      <div>Expiry at {{ props.countdown.expiryTimestamp.toLocaleTimeString() }}</div>
    </details>
  </div>
</template>

<style lang="css" scoped>
div * + * {
  margin-left: 0.2em;
}
</style>
