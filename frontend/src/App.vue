<script setup lang="ts">
import { useTemplateRef } from 'vue'
import CountdownList from './CountdownList.vue'
import { addCountdown } from './countdowns.ts'
import { notifs } from './notifs.ts'
import DismissModal from './DismissModal.vue'

const inputRef = useTemplateRef('input')
const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')

const onSubmit = () => {
  if (inputRef.value === null || listRef.value === null || nameInputRef.value === null) {
    return
  }
  const date = new Date(inputRef.value.value)
  if (Number.isNaN(date.getTime())) {
    return
  }
  addCountdown({
    expiryTimestamp: date,
    startedAt: new Date(),
    name: nameInputRef.value.value || undefined,
  })
}
</script>

<template>
  <form @submit.prevent="onSubmit" style="display: flex; align-items: center;">
    <span style="margin-right: 1em;">Add Alarm:</span>
    <input type="text" ref="name-input" placeholder="Name" />
    <input type="text" ref="input" placeholder="Time" />
    <button>Submit</button>
  </form>
  <CountdownList ref="list" />
  <DismissModal v-if="notifs.size > 0"></DismissModal>
</template>

<style scoped>
form > * + * {
  margin-left: 0.2em;
}
</style>
