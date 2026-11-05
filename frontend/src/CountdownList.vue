<script setup lang="ts">
import { onBeforeUnmount, ref, watchSyncEffect, type Ref } from 'vue'
import TheCountdown from './TheCountdown.vue'
import { countdownEventTarget, countdowns, type Countdown } from './countdowns.ts'

const emit = defineEmits<{ expired: [Countdown] }>()
/** only contains ids of countdowns in {@link countdownEls} */
const countdownInactive = ref<Set<number>>(new Set())
const countdownEls = ref<Map<number, InstanceType<typeof TheCountdown>>>(new Map())

const visibilityChangeListenerAc = new AbortController()
const isUnmounted = ref(false)
const documentVisibility = ref(true)

const detailsName = String(Math.random())

let clockCallbackId: number | null = null

function clock() {
  countdownEls.value.forEach((cd) => cd.update())
  const now = Date.now()
  const nextSecond = Math.ceil(now / 1000) * 1000
  clockCallbackId = setTimeout(clock, nextSecond - now)
}

addEventListener(
  'visibilitychange',
  () => {
    documentVisibility.value = document.visibilityState !== 'hidden'
  },
  {
    signal: visibilityChangeListenerAc.signal,
  },
)

watchSyncEffect(() => {
  const clockShouldRun =
    documentVisibility.value &&
    !isUnmounted.value &&
    countdownInactive.value.size < countdownEls.value.size
  if (clockCallbackId === null) {
    if (clockShouldRun) {
      clock()
    }
  } else {
    if (!clockShouldRun) {
      clearTimeout(clockCallbackId)
      clockCallbackId = null
    }
  }
})

const onCountdownRemoved = (cd: Countdown) => {
  countdownInactive.value.delete(cd.id)
}
countdownEventTarget.addEventListener('removed', onCountdownRemoved)
const onCountdownExpired = async (countdown: Countdown) => {
  countdownInactive.value.add(countdown.id)
  countdownEls.value.get(countdown.id)?.update()
  emit('expired', countdown)
}
countdownEventTarget.addEventListener('expired', onCountdownExpired)
const onCountdownPaused = async (countdown: Countdown) => {
  countdownInactive.value.add(countdown.id)
  countdownEls.value.get(countdown.id)?.update()
}
countdownEventTarget.addEventListener('paused', onCountdownPaused)
const onCountdownContinued = async (countdown: Countdown) => {
  countdownInactive.value.delete(countdown.id)
  countdownEls.value.get(countdown.id)?.update()
}
countdownEventTarget.addEventListener('continued', onCountdownContinued)

onBeforeUnmount(() => {
  isUnmounted.value = true
  visibilityChangeListenerAc.abort()
  countdownEventTarget.removeEventListener('removed', onCountdownRemoved)
  countdownEventTarget.removeEventListener('expired', onCountdownExpired)
  countdownEventTarget.addEventListener('continued', onCountdownContinued)
  countdownEventTarget.addEventListener('paused', onCountdownPaused)
})
</script>

<template>
  <div>
    <TheCountdown
      :details-name="detailsName"
      :countdown
      v-for="[id, countdown] in countdowns"
      :key="id"
      :ref="
        (cd) => {
          if (cd === null) countdownEls.delete(id)
          else countdownEls.set(id, cd as any)
        }
      "
      style="margin: 0.5em 0"
    />
  </div>
</template>

<style scoped></style>
