<script setup lang="ts">
import { closeAllNotifications, notifs } from './notifs'
import { audio } from './audio'
import { onBeforeMount, onMounted, useTemplateRef } from 'vue'

const dialog = useTemplateRef('dialog')
onBeforeMount(() => {
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur()
  }
})
onMounted(() => {
  dialog.value?.showModal()
})
</script>

<template>
  <dialog
    closedby="none"
    ref="dialog"
    style="
      background-color: var(--color-primary);
      width: 40em;
      height: 20em;
      padding: 0 1em;
      border-style: none;
      border-radius: var(--border-radius-small);
      box-shadow: 0px 0px 3px 0px var(--color-primary);
      color: var(--color-text);
    "
    @close="
      () => {
        audio.stop()
        closeAllNotifications()
      }
    "
  >
    <h1>Expired alarms</h1>
    <ul>
      <li v-for="[id, { countdown: cd }] in notifs" :key="id">
        {{ cd.name ?? 'Unnamed' }}: Expired at {{ cd.expiryTimestamp.toLocaleTimeString() }}
      </li>
    </ul>
    <button
      @click="
        () => {
          dialog?.close()
        }
      "
      autofocus
    >
      Dismiss
    </button>
  </dialog>
</template>

<style scoped>
::backdrop {
  background-color: hsl(from var(--color-text) h s l / 0.3);
}
</style>
