<script setup lang="ts">
import { onMounted, onUnmounted, ref, useTemplateRef, type Ref } from 'vue'
import CountdownList from './CountdownList.vue'
import { notifs } from './notifs.ts'
import DismissModal from './DismissModal.vue'
import { Connection } from './connection.ts'
import { Context } from './context.ts'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const inputRef = useTemplateRef('input')
const dateInputAsDate: Ref<Date | undefined> = ref(undefined)
const submitting: Ref<boolean> = ref(false)
const ctx = Context.background()

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
  await Connection.getInstance().createAlarm(ctx, expiryTimestamp, new Date(), nameInputRef.value.value || undefined)
  submitting.value = false
}

onMounted(() => {
  const now = new Date()
  now.setSeconds(0)
  now.setMilliseconds(0)
  inputRef.value!.value = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
})

onUnmounted(() => {
  ctx.abort()
})
</script>

<template>
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
  <DismissModal v-if="notifs.size > 0"></DismissModal>
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
