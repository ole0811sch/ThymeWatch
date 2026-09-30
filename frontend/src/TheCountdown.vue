<script setup lang="ts">
import { onUnmounted, ref, useTemplateRef } from 'vue'
import {
  removeCountdown,
  continueCountdown,
  countdownEventTarget,
  pauseCountdown,
  type Countdown,
} from './countdowns'
const { countdown, detailsName } = defineProps<{ countdown: Countdown, detailsName: string }>()

const displayText = ref('')
const isPaused = ref(countdown.pausedAt !== undefined)
const isExpired = ref(countdown.expired)
const details = useTemplateRef('details')

function update() {
  if (!countdown.expiryTimestamp) {
    displayText.value = ''
    return
  }
  const currentTime = countdown.pausedAt?.getTime() ?? Date.now()
  const timeLeft = Math.max(countdown.expiryTimestamp.getTime() - currentTime, 0)
  const secondsLeft = Math.round(timeLeft / 1000)
  const minutesLeft = Math.floor(secondsLeft / 60)
  const hoursLeft = Math.floor(minutesLeft / 60)
  const secondsField = secondsLeft % 60
  const minutesField = minutesLeft % 60
  const hoursText = hoursLeft ? `${String(hoursLeft).padStart(2, '0')}:` : ''
  displayText.value = `${hoursText}${String(minutesField).padStart(2, '0')}:${String(secondsField).padStart(2, '0')}`
}

const onExpired = (_cd: Countdown) => {
  isExpired.value = true
  update()
}
countdownEventTarget.addEventListener('expired', onExpired, { countdownId: countdown.id })
const onPaused = () => {
  isPaused.value = true
}
countdownEventTarget.addEventListener('paused', onPaused, { countdownId: countdown.id })
const onContinued = () => {
  isPaused.value = false
}
countdownEventTarget.addEventListener('continued', onContinued, { countdownId: countdown.id })

const onRequestRemove = () => {
  removeCountdown(countdown.id)
}

const onRequestTogglePause = () => {
  if (isPaused.value) {
    continueCountdown(countdown.id)
  } else {
    pauseCountdown(countdown.id)
  }
}

update()

onUnmounted(() => {
  countdownEventTarget.removeEventListener('expired', onExpired, { countdownId: countdown.id })
  countdownEventTarget.removeEventListener('paused', onPaused, { countdownId: countdown.id })
  countdownEventTarget.removeEventListener('continued', onContinued, { countdownId: countdown.id })
})

defineExpose({ update })
</script>

<template>
  <div style="display: flex; align-items: center" class="main">
    <div :class="[...(isExpired ? ['expired'] : isPaused ? ['paused'] : []), 'timer']">
      {{ displayText }}
    </div>
    <button @click="onRequestRemove">Remove</button>
    <button @click="onRequestTogglePause" v-if="!isExpired">
      {{ isPaused ? 'Continue' : 'Pause' }}
    </button>
    <details style="display: inline" ref="details" :name="detailsName">
      <summary style="cursor: pointer">Show details</summary>
      <div style="position: relative">
        <div
          style="
            position: absolute;
            top: 0.2em;
            background-color: var(--color-secondary);
            border-radius: var(--border-radius-smaller);
            padding: var(--box-padding);
            width: max-content;
          "
        >
          <div v-if="countdown.name">Name: {{ countdown.name }}</div>
          <div>Started at {{ countdown.startedAt.toLocaleTimeString() }}</div>
          <div v-if="!isPaused">Expiry at {{ countdown.expiryTimestamp.toLocaleTimeString() }}</div>
          <div
            v-if="
              isPaused ||
              countdown.originalExpiryTimestamp.getTime() !== countdown.expiryTimestamp.getTime()
            "
          >
            Initial expiry at
            {{ countdown.originalExpiryTimestamp.toLocaleTimeString() }}
          </div>
        </div>
      </div>
    </details>
  </div>
</template>

<style lang="css" scoped>
div.main > * + * {
  margin-left: 0.2em;
}
.timer {
  font-size: var(--font-size-big);
  background-color: var(--color-secondary);
  border-radius: var(--border-radius-smaller);
  padding: var(--box-padding);
}

.timer.expired {
  background-color: var(--color-contrast);
}

.timer.paused {
  background-color: var(--color-timer-paused);
}
</style>
