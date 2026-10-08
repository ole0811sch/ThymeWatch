<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef, type Ref } from 'vue'
import CountdownList from './CountdownList.vue'
import { addCountdown } from './countdowns.ts'
import { notifs } from './notifs.ts'
import DismissModal from './DismissModal.vue'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const inputRef = useTemplateRef('input')
const dateInputAsDate: Ref<Date | undefined> = ref(undefined)

const onSubmit = () => {
  if (listRef.value === null || nameInputRef.value === null) {
    return
  }
  if (dateInputAsDate.value === undefined || Number.isNaN(dateInputAsDate.value.getTime())) {
    return
  }
  const expiryTimestamp = new Date()
  expiryTimestamp.setHours(0)
  expiryTimestamp.setMinutes(0)
  expiryTimestamp.setSeconds(0)
  expiryTimestamp.setMilliseconds(0)
  expiryTimestamp.setTime(expiryTimestamp.getTime() + dateInputAsDate.value.getTime())
  addCountdown({
    expiryTimestamp,
    startedAt: new Date(),
    name: nameInputRef.value.value || undefined,
  })
}

onMounted(() => {
  const now = new Date()
  now.setSeconds(0)
  now.setMilliseconds(0)
  inputRef.value!.value = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
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
      <button :disabled="dateInputAsDate === undefined || isNaN(dateInputAsDate.getTime())">
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
