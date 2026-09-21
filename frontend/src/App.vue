<script setup lang="ts">
import { ref, useTemplateRef } from 'vue'
import CountdownList, { type Countdown } from './CountdownList.vue'

const inputRef = useTemplateRef('input')
const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const countdowns = ref<{ countdowns: Countdown[] }>({ countdowns: [] })
Notification.requestPermission()

const onSubmit = () => {
  if (inputRef.value === null || listRef.value === null || nameInputRef.value === null) {
    return
  }
  const date = new Date(inputRef.value.value)
  if (Number.isNaN(date.getTime())) {
    return
  }
  listRef.value.pushCountdown({
    expiryTimestamp: date,
    startedAt: new Date(),
    name: nameInputRef.value.value || undefined,
  })
}

const onExpired = async (countdown: Countdown) => {
  const title = countdown.name ? `Timer expired: ${countdown.name}` : 'Timer expired'
  if (Notification.permission === 'granted') {
    new Notification(title)
  } else {
    alert(title)
  }
}
</script>

<template>
  <form @submit.prevent="onSubmit">
    <input type="text" ref="name-input" placeholder="Name" />
    <input type="text" ref="input" placeholder="Time" />
    <button>Submit</button>
  </form>
  <CountdownList v-model="countdowns" ref="list" @expired="onExpired" />
</template>

<style scoped></style>
