<script setup lang="ts">
import { useTemplateRef } from 'vue'
import CountdownList from './CountdownList.vue'
import { addCountdown } from './countdowns.ts'

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
  <form @submit.prevent="onSubmit">
    <input type="text" ref="name-input" placeholder="Name" />
    <input type="text" ref="input" placeholder="Time" />
    <button>Submit</button>
  </form>
  <CountdownList ref="list" />
</template>

<style scoped></style>
