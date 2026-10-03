<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useTemplateRef, watch, type Ref } from 'vue'
import CountdownList from './CountdownList.vue'
import { addCountdown } from './countdowns.ts'
import { notifs } from './notifs.ts'
import DismissModal from './DismissModal.vue'
import { Connection } from './test.ts'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const dateInput: Ref<string> = ref('')
const dateInputAsDate = computed(() => new Date(dateInput.value))
const connectionProm = Connection.getInstance()
let connection: Connection | null = null
const isConnected: Ref<'uninitialized' | 'connecting' | 'open' | 'closed'> = ref('uninitialized')

connectionProm.then((conn) => {
  connection = conn
  isConnected.value = connection.state
  connection.addEventListener('closed', () => (isConnected.value = 'closed'))
  connection.addEventListener('reconnected', () => (isConnected.value = 'open'))
})
const onWindowFocus = () => {
  if (connection?.state === 'closed') {
    connection.reconnect()
  }
}
addEventListener('focus', onWindowFocus)
const onClickRetry = () => {
  isConnected.value = 'connecting'
  connection?.reconnect()
}

const onSubmit = () => {
  if (listRef.value === null || nameInputRef.value === null) {
    return
  }
  if (Number.isNaN(dateInputAsDate.value.getTime())) {
    return
  }
  addCountdown({
    expiryTimestamp: dateInputAsDate.value,
    startedAt: new Date(),
    name: nameInputRef.value.value || undefined,
  })
}
onBeforeUnmount(() => {
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
    v-if="isConnected === 'connecting' || isConnected === 'closed'"
  >
    <span style="margin-right: 0.5em"> The connection to the server was lost. </span>
    <button
      :style="{
        display: 'inline',
        ...(isConnected === 'connecting' && {
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
    <span v-if="isConnected === 'connecting'">Reconnecting...</span>
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
      <input type="text" ref="input" placeholder="Time" v-model="dateInput" />
      <button :disabled="isNaN(dateInputAsDate.getTime())">Submit</button>
    </form>
    <div style="flex: 1"></div>
    <div style="display: flex; flex-direction: column; justify-content: center">
      <CountdownList ref="list" style="height: fit-content" />
    </div>
    <div style="flex: 2"></div>
  </div>
  <DismissModal v-if="notifs.size > 0"></DismissModal>
</template>

<style scoped>
form > * + * {
  margin-left: 0.2em;
}
</style>
