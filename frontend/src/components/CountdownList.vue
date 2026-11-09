<script setup lang="ts">
import { onBeforeUnmount, ref, watchSyncEffect, type Ref } from 'vue'
import TheCountdown from '../components/TheCountdown.vue'
import { countdownEventTarget, countdowns, type Countdown } from '../countdowns.ts'
import { addEventListenerWithLifetime } from '../utils/componentLifetimeEventListener.ts'

const emit = defineEmits<{ expired: [Countdown] }>()
/** only contains ids of countdowns in {@link countdownEls} */
const countdownInactive = ref<Set<number>>(new Set())
const countdownEls = ref<Map<number, InstanceType<typeof TheCountdown>>>(new Map())

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

addEventListenerWithLifetime(onBeforeUnmount, window, 'visibilitychange', () => {
  documentVisibility.value = document.visibilityState !== 'hidden'
})

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

addEventListenerWithLifetime(onBeforeUnmount, countdownEventTarget, 'removed', (cd: Countdown) => {
  countdownInactive.value.delete(cd.id)
})
addEventListenerWithLifetime(
  onBeforeUnmount,
  countdownEventTarget,
  'expired',
  async (countdown: Countdown) => {
    countdownInactive.value.add(countdown.id)
    countdownEls.value.get(countdown.id)?.update()
    emit('expired', countdown)
  },
)
addEventListenerWithLifetime(
  onBeforeUnmount,
  countdownEventTarget,
  'paused',
  async (countdown: Countdown) => {
    countdownInactive.value.add(countdown.id)
    countdownEls.value.get(countdown.id)?.update()
  },
)
addEventListenerWithLifetime(
  onBeforeUnmount,
  countdownEventTarget,
  'continued',
  async (countdown: Countdown) => {
    countdownInactive.value.delete(countdown.id)
    countdownEls.value.get(countdown.id)?.update()
  },
)

onBeforeUnmount(() => {
  isUnmounted.value = true
})
</script>

<template>
  <div style="display: flex; flex-wrap: wrap; justify-content: center;">
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
      style="margin: 0.5em 0.2em"
    />
  </div>
</template>

<style scoped></style>
