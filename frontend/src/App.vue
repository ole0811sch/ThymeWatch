<script setup lang="ts">
import { computed, onUnmounted, ref, type Ref } from 'vue'
import { Connection } from './connection'
import { debounceRef, temporarilyFixRef } from './utils/debounce'

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
  <div class="page-container">
    <RouterView></RouterView>
  </div>
</template>
