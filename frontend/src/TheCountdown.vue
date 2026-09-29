<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import { countdownEventTarget, type Countdown } from './countdowns';
const { countdown } = defineProps<{ countdown: Countdown }>()
const emit = defineEmits<{ requestRemove: []; }>()

const displayText = ref('')

function update() {
  if (!countdown.expiryTimestamp) {
    displayText.value = ''
    return
  }
  const timeLeft = Math.max(countdown.expiryTimestamp.getTime() - Date.now(), 0)
  const secondsLeft = Math.round(timeLeft / 1000)
  const minutesLeft = Math.floor(secondsLeft / 60)
  const hoursLeft = Math.floor(minutesLeft / 60)
  const secondsField = secondsLeft % 60
  const minutesField = minutesLeft % 60
  const hoursText = hoursLeft ? `${String(hoursLeft).padStart(2, '0')}:` : ''
  displayText.value = `${hoursText}${String(minutesField).padStart(2, '0')}:${String(secondsField).padStart(2, '0')}`
}

const onExpired = (_cd: Countdown) => {
  update();
}
countdownEventTarget.addEventListener("expired", onExpired, { countdownId: countdown.id })
onUnmounted(() => {
  countdownEventTarget.removeEventListener("expired", onExpired, { countdownId: countdown.id })
})

update()

defineExpose({ update })
</script>

<template>
  <div style="display: flex; align-items: start">
    <span>{{ displayText }}</span>
    <button @click="$emit('requestRemove')">Remove</button>
    <details :style="{ display: 'inline' }">
      <summary>Show details</summary>
      <div v-if="countdown.name">Name: {{ countdown.name }}</div>
      <div>Started at {{ countdown.startedAt.toLocaleTimeString() }}</div>
      <div>Expiry at {{ countdown.expiryTimestamp.toLocaleTimeString() }}</div>
    </details>
  </div>
</template>

<style lang="css" scoped>
div *+* {
  margin-left: 0.2em;
}
</style>
