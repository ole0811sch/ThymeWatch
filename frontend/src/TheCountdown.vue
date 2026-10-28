<script setup lang="ts">
import { onUnmounted, ref, useTemplateRef } from 'vue'
import { countdownEventTarget, type Countdown } from './countdowns'
import { Connection } from './connection'
import { Abort, Context } from './context'
const { countdown, detailsName } = defineProps<{ countdown: Countdown; detailsName: string }>()

const displayText = ref('')
const isPaused = ref(countdown.pausedAtTime !== null)
const isExpired = ref(countdown.isExpired)
const details = useTemplateRef('details')
const ctx = Context.background()

function update() {
  if (!countdown.expiryTimestamp) {
    displayText.value = ''
    return
  }
  const currentTime = countdown.pausedAtTime?.getTime() ?? Date.now()
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

const removing = ref(false)
const togglingPause = ref(false)

const onRequestRemove = async () => {
  removing.value = true
  const ctx2 = Context.withTimeout(5000, ctx)
  const conn = Connection.getInstance()
  const response = await conn.cancelAlarm(ctx2, countdown.id)
  if (response instanceof Error || response instanceof Abort) {
    console.error('Could not cancel alarm', response)
  }
  removing.value = false
  ctx2.cancel()
}

const onRequestTogglePause = async () => {
  togglingPause.value = true
  try {
    const ctx2 = Context.withTimeout(5000, ctx)
    const conn = Connection.getInstance()
    if (isPaused.value) {
      conn.continueAlarm(ctx2, countdown.id)
    } else {
      conn.pauseAlarm(ctx2, countdown.id)
    }
    ctx2.cancel()
  } finally {
    togglingPause.value = false
  }
}

const handlerTargetOutisdeDetails = (e: Event) => {
  if ((e.target instanceof Node && details.value?.contains(e.target)) || details.value === null) {
    return
  }
  details.value.open = false
}

const handlers = [
  ['pointerdown', handlerTargetOutisdeDetails],
  ['focusin', handlerTargetOutisdeDetails],
] as const
const onDetailsToggle = (e: ToggleEvent) => {
  if (e.oldState === 'closed') {
    for (const [type, handler] of handlers) {
      addEventListener(type, handler)
    }
    if (details.value) details.value.open = true
  } else {
    for (const [type, handler] of handlers) {
      removeEventListener(type, handler)
    }
    if (details.value) details.value.open = false
  }
}

update()

onUnmounted(() => {
  ctx.abort()
  countdownEventTarget.removeEventListener('expired', onExpired, { countdownId: countdown.id })
  countdownEventTarget.removeEventListener('paused', onPaused, { countdownId: countdown.id })
  countdownEventTarget.removeEventListener('continued', onContinued, { countdownId: countdown.id })
  for (const [type, handler] of handlers) {
    removeEventListener(type, handler)
  }
})

defineExpose({ update })
</script>

<template>
  <div style="display: flex; align-items: center" class="main">
    <div :class="[...(isExpired ? ['expired'] : isPaused ? ['paused'] : []), 'timer']">
      {{ displayText }}
    </div>
    <button @click="onRequestRemove" :disabled="removing">Remove</button>
    <button @click="onRequestTogglePause" v-if="!isExpired">
      {{ isPaused ? 'Continue' : 'Pause' }}
    </button>
    <details style="display: inline" ref="details" :name="detailsName" @toggle="onDetailsToggle">
      <summary style="cursor: pointer"><span style="user-select: none">Show details</span></summary>
      <div style="position: relative" ref="details-content">
        <div
          style="
            position: absolute;
            left: 0.3em;
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
  user-select: none;
}

.timer.expired {
  background-color: var(--color-contrast);
}

.timer.paused {
  background-color: var(--color-timer-paused);
}
</style>
