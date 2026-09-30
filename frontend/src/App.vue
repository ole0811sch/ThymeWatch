<script setup lang="ts">
import { computed, ref, useTemplateRef, type Ref } from 'vue'
import CountdownList from './CountdownList.vue'
import { addCountdown } from './countdowns.ts'
import { notifs } from './notifs.ts'
import DismissModal from './DismissModal.vue'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const dateInput: Ref<string> = ref("");
const dateInputAsDate = computed(() => new Date(dateInput.value))

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
</script>

<template>
  <form @submit.prevent="onSubmit" style="display: flex; align-items: center;">
    <span style="margin-right: 1em; user-select: none;">Add Alarm:</span>
    <input type="text" ref="name-input" placeholder="Name" />
    <input type="text" ref="input" placeholder="Time" v-model="dateInput" />
    <button :disabled="isNaN(dateInputAsDate.getTime())">Submit</button>
  </form>
  <CountdownList ref="list" />
  <DismissModal v-if="notifs.size > 0"></DismissModal>
</template>

<style scoped>
form > * + * {
  margin-left: 0.2em;
}
</style>
