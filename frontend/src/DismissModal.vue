<script setup lang="ts">
import { computed, onBeforeMount, onMounted, onUnmounted, useTemplateRef } from 'vue'
import { Connection as Connection2 } from './connection'
import { Context } from './context'
import { countdowns, expiredUndismissedAlarms } from './countdowns'

const Connection = Connection2
const dialog = useTemplateRef('dialog')
const ctx = Context.background()
const expiredUndismissedAlarmObjs = computed(() =>
  [...expiredUndismissedAlarms.value.values()]
    .map((id) => countdowns.value.get(id))
    .filter((cd) => cd !== undefined)
    .sort((a, b) =>
      a.expiryTimestamp.getTime() < b.expiryTimestamp.getTime()
        ? -1
        : a.expiryTimestamp.getTime() === b.expiryTimestamp.getTime()
          ? 0
          : 1,
    ),
)
onBeforeMount(() => {
  if (document.activeElement instanceof HTMLElement) {
    document.activeElement.blur()
  }
})
onMounted(() => {
  dialog.value?.showModal()
})
onUnmounted(() => {
  ctx.abort()
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
      async () => {
        await Connection.getInstance().dismissAlarms(ctx, [...expiredUndismissedAlarms.values()])
      }
    "
  >
    <h1>Expired alarms</h1>
    <ul>
      <li v-for="cd in expiredUndismissedAlarmObjs" :key="cd.id">
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
