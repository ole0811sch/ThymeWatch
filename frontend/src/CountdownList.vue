<script setup lang="ts">
import { onBeforeUnmount, ref, toRaw, toRefs, watch, watchSyncEffect, type Ref } from 'vue'
import TheCountdown from './TheCountdown.vue'
import { countdownEventTarget, getCountdowns, type Countdown } from './countdowns.ts';

const countdowns: Ref<Map<string, Countdown>> = ref(getCountdowns());
const emit = defineEmits<{ expired: [Countdown] }>()
/** only contains ids of countdowns in {@link countdownEls} */
const countdownExpired = ref<Record<string, true>>({})
const countdownEls = ref<Record<string, InstanceType<typeof TheCountdown>>>({})

const visibilityChangeListenerAc = new AbortController()
const isUnmounted = ref(false)
const documentVisibility = ref(true)
let clockCallbackId: number | null = null

function clock() {
  Object.values(countdownEls.value).forEach((cd) => cd.update())
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
    Object.values(countdownExpired.value).length < Object.values(countdownEls.value).length
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

const onRequestRemove = (id: string) => {
  countdowns.value.get(id)?.abortController.abort();
}

const onCountdownCancel = (cd: Countdown) => {
  delete countdownExpired.value[cd.id]
  countdowns.value.delete(cd.id)
}
countdownEventTarget.addEventListener("cancelled", onCountdownCancel)
const onNewCountdown = (cd: Countdown) => {
  countdowns.value.set(cd.id, cd)
}
countdownEventTarget.addEventListener("new", onNewCountdown)
const onCountdownExpired = async (countdown: Countdown) => {
  countdownExpired.value[countdown.id] = true
  countdownEls.value[countdown.id]?.update()
  emit('expired', countdown)
}
countdownEventTarget.addEventListener("expired", onCountdownExpired)


onBeforeUnmount(() => {
  isUnmounted.value = true
  visibilityChangeListenerAc.abort()
  countdownEventTarget.removeEventListener("cancelled", onCountdownCancel)
  countdownEventTarget.removeEventListener("new", onNewCountdown)
  countdownEventTarget.removeEventListener("expired", onCountdownExpired)
})
</script>

<template>
  <TheCountdown :countdown @request-remove="onRequestRemove(countdown.id ?? '')" v-for="[id, countdown] in countdowns"
    :key="id" :ref="(cd) => {
        if (cd === null) delete countdownEls[id]
        else countdownEls[id] = cd as any
      }
      " />
</template>

<style scoped></style>
