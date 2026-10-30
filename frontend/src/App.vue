<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch, type Ref } from 'vue'
import CountdownList from './CountdownList.vue'
import DismissModal from './DismissModal.vue'
import { Connection } from './connection.ts'
import { Context } from './context.ts'
import { debounceRef, temporarilyFixRef } from './debounce.ts'
import { expiredUndismissedAlarms } from './countdowns.ts'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const inputRef = useTemplateRef('input')
const dateInputAsDate: Ref<Date | undefined> = ref(undefined)
const submitting: Ref<boolean> = ref(false)
const ctx = Context.background()
const isConnected: Ref<'uninitialized' | Connection['state']> = ref('uninitialized')
const connection = Connection.getInstance()
// prevent from toggling back to false too quickly
const hideShowReconnectBtn = debounceRef(
  computed(() => isConnected.value === 'connecting' || isConnected.value === 'open'),
  300,
  { doUpdateImmediately: (v) => v },
)
const doShowDisconnectedBar = debounceRef(
  temporarilyFixRef(
    computed(() => isConnected.value === 'connecting' || isConnected.value === 'closed'),
    false,
    2000,
  ),
  1000,
)

isConnected.value = connection.state
connection.addEventListener('closed', () => (isConnected.value = 'closed'))
connection.addEventListener('reconnected', () => (isConnected.value = 'open'))
const onSubmit = async () => {
  if (listRef.value === null || nameInputRef.value === null) {
    return
  }
  if (dateInputAsDate.value === undefined || Number.isNaN(dateInputAsDate.value.getTime())) {
    return
  }
  const expiryTimestamp = new Date()
  const expiryTimestampOrig = new Date(expiryTimestamp)
  expiryTimestamp.setHours(dateInputAsDate.value.getUTCHours())
  expiryTimestamp.setMinutes(dateInputAsDate.value.getUTCMinutes())
  expiryTimestamp.setSeconds(dateInputAsDate.value.getUTCSeconds())
  expiryTimestamp.setMilliseconds(dateInputAsDate.value.getUTCMilliseconds())
  if (expiryTimestamp.getTime() < expiryTimestampOrig.getTime()) {
    expiryTimestamp.setDate(expiryTimestamp.getDate() + 1)
  }
  submitting.value = true
  await Connection.getInstance().createAlarm(
    ctx,
    expiryTimestamp,
    new Date(),
    nameInputRef.value.value || undefined,
  )
  submitting.value = false
}

onMounted(() => {
  const now = new Date()
  now.setSeconds(0)
  now.setMilliseconds(0)
  inputRef.value!.value = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
})

const onWindowFocus = () => {
  if (Connection.getInstance().state === 'closed') {
    Connection.getInstance().reconnect()
  }
}
addEventListener('focus', onWindowFocus)
const onClickRetry = () => {
  isConnected.value = 'connecting'
  Connection.getInstance().reconnect()
}

onUnmounted(() => {
  ctx.abort()
  removeEventListener('focus', onWindowFocus)
})
</script>

<template>
  <div
    style="
      display: flex;
      align-items: center;
      background-color: var(--color-contrast-intensish);
      width: 100%;
      padding: var(--box-padding);
      box-sizing: border-box;
    "
    v-if="doShowDisconnectedBar"
  >
    <span style="margin-right: 0.5em"> The connection to the server was lost. </span>
    <button
      :style="{
        display: 'inline',
        ...(hideShowReconnectBtn && {
          visibility: 'hidden',
          width: 0,
          paddingLeft: 0,
          paddingRight: 0,
          marginLeft: 0,
          marginRight: 0,
        }),
      }"
      @click="onClickRetry"
    >
      Reconnect
    </button>
    <span v-if="hideShowReconnectBtn">Reconnecting...</span>
    <span style="flex: 1"></span>
  </div>
  <div
    style="
      margin-left: auto;
      margin-right: auto;
      margin-top: 1em;
      width: fit-content;
      flex: 1;
      display: flex;
      flex-direction: column;
    "
  >
    <form @submit.prevent="onSubmit" style="display: flex; align-items: center; margin-bottom: 2em">
      <span style="margin-right: 1em; user-select: none">Add Alarm:</span>
      <input type="text" spellcheck="false" ref="name-input" placeholder="Name" />
      <input
        type="time"
        ref="input"
        placeholder="Time"
        @input="() => (dateInputAsDate = inputRef?.valueAsDate ?? undefined)"
      />
      <button
        :disabled="dateInputAsDate === undefined || isNaN(dateInputAsDate.getTime()) || submitting"
      >
        Submit
      </button>
    </form>
    <div style="flex: 1"></div>
    <div style="display: flex; flex-direction: column; justify-content: center">
      <CountdownList ref="list" style="height: fit-content" />
    </div>
    <div style="flex: 2"></div>
  </div>
  <DismissModal v-if="expiredUndismissedAlarms.size > 0"></DismissModal>
</template>

<style scoped>
form {
  & > * + * {
    margin-left: 0.2em;
  }
  & > input {
    min-height: 1.5em;
  }
}
</style>
