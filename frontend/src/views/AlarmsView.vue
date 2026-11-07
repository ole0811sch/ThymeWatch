<script setup lang="ts">
import { onMounted, onUnmounted, ref, useTemplateRef, type Ref } from 'vue'
import CountdownList from '../components/CountdownList.vue'
import DismissModal from '../components/DismissModal.vue'
import { Connection } from '../connection.ts'
import { Context } from '../utils/context.ts'
import { expiredUndismissedAlarms } from '../countdowns.ts'
import { paths } from '@/router/index.ts'

const nameInputRef = useTemplateRef('name-input')
const listRef = useTemplateRef('list')
const inputRef = useTemplateRef('input')
const dateInputAsDate: Ref<Date | undefined> = ref(undefined)
const submitting: Ref<boolean> = ref(false)
const ctx = Context.withCancel<'unmounted'>()

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
  await Connection.getInstance().createAlarm(
    ctx,
    expiryTimestamp,
    new Date(),
    nameInputRef.value.value || undefined,
  )
  submitting.value = false
}

onMounted(() => {
  const now = new Date()
  now.setSeconds(0)
  now.setMilliseconds(0)
  inputRef.value!.value = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
})

onUnmounted(() => {
  ctx.abort('unmounted')
})
</script>

<template>
  <div
    class="page-container"
    style="
      width: 100%;
      box-sizing: border-box;
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
    "
  >
    <div style="display: flex; align-items: center; margin-bottom: 2em; width: 100%">
      <div style="flex: 1"></div>
      <form @submit.prevent="onSubmit" style="display: flex; align-items: center">
        <span style="margin-right: 1em; user-select: none">Add Alarm:</span>
        <input type="text" spellcheck="false" ref="name-input" placeholder="Name" />
        <input
          type="time"
          ref="input"
          placeholder="Time"
          @input="() => (dateInputAsDate = inputRef?.valueAsDate ?? undefined)"
        />
        <button
          :disabled="
            dateInputAsDate === undefined || isNaN(dateInputAsDate.getTime()) || submitting
          "
        >
          Submit
        </button>
      </form>
      <div style="flex: 1; display: flex; justify-content: right">
        <button @click="$router.push(paths.settings)">Settings</button>
      </div>
    </div>
    <div style="flex: 1"></div>
    <div style="display: flex; flex-direction: column; justify-content: center">
      <CountdownList ref="list" style="height: fit-content" />
    </div>
    <div style="flex: 2"></div>
  </div>
  <DismissModal v-if="expiredUndismissedAlarms.size > 0"></DismissModal>
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
