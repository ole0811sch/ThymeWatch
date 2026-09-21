<script setup lang="ts">
import { onBeforeUnmount, ref, toRaw, toRefs, watch, watchSyncEffect } from 'vue'
import TheCountdown from './TheCountdown.vue'

export interface CountdownInput {
  startedAt: Date
  expiryTimestamp: Date
  name?: string
  id?: string
}
export interface Countdown extends CountdownInput {
  id: string
}

const model = defineModel<{
  countdowns: Countdown[]
}>({
  required: true,
})
const emit = defineEmits<{ expired: [Countdown] }>()
/** only contains ids of countdowns in {@link countdowns} */
const countdownExpired = ref<Record<string, true>>({})
const countdowns = ref<Record<string, InstanceType<typeof TheCountdown>>>({})

const onExpired = async (countdown: Countdown) => {
  countdownExpired.value[countdown.id] = true
  countdowns.value[countdown.id]?.update()
  emit('expired', countdown)
}

const visibilityChangeListenerAc = new AbortController()
const isUnmounted = ref(false)
const documentVisibility = ref(true)
let clockCallbackId: number | null = null

function clock() {
  Object.values(countdowns.value).forEach((cd) => cd.update())
  const now = Date.now()
  const nextSecond = Math.ceil(now / 1000) * 1000
  clockCallbackId = setTimeout(clock, nextSecond - now)
}

onBeforeUnmount(() => {
  isUnmounted.value = true
  visibilityChangeListenerAc.abort()
})
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
    Object.values(countdownExpired.value).length < Object.values(countdowns.value).length
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
  delete countdownExpired.value[id]
  if (model.value) {
    model.value.countdowns = model.value.countdowns.filter((cd) => cd.id !== id)
  }
}

function pushCountdown(cd: CountdownInput) {
  model.value.countdowns.push({
    ...cd,
    id: cd.id ?? crypto.randomUUID(),
  })
}

defineExpose({ pushCountdown })
</script>

<template>
  <TheCountdown
    :countdown
    @request-remove="onRequestRemove(countdown.id ?? '')"
    @expired="onExpired"
    v-for="countdown in model.countdowns"
    :key="countdown.id"
    :ref="
      (cd) => {
        if (cd === null) delete countdowns[countdown.id]
        else countdowns[countdown.id] = cd as any
      }
    "
  />
</template>

<style scoped></style>
