<script setup lang="ts">
import { onUnmounted, ref, useTemplateRef } from 'vue'
import { countdownEventTarget, type Countdown } from '../countdowns'
import { Connection } from '../connection'
import { Abort, Context } from '../utils/context'
import { addEventListenerWithLifetime } from '../utils/componentLifetimeEventListener'
import IconButton from './IconButton.vue'
import PauseIcon from '@iconify-vue/ic/round-pause'
import CancelIcon from '@iconify-vue/ic/round-cancel'
import PlayArrowIcon from '@iconify-vue/ic/round-play-arrow'
import DetailsIcon from '@iconify-vue/ic/round-info'
import ReplayIcon from '@iconify-vue/ic/round-replay'

const { countdown, detailsName } = defineProps<{ countdown: Countdown; detailsName: string }>()

const displayText = ref('')
const dialog = useTemplateRef('dialog')
const ctx = Context.withCancel<'unmounted'>()

function update() {
  if (!countdown.expiryTimestamp) {
    displayText.value = ''
    return
  }
  const currentTime = countdown.pausedAtTime?.getTime() ?? Date.now()
  const timeLeft = Math.max(countdown.expiryTimestamp.getTime() - currentTime, 0)
  const secondsLeft = Math.round(timeLeft / 1000)
  const minutesLeft = Math.floor(secondsLeft / 60)
  const hoursLeft = Math.floor(minutesLeft / 60)
  const secondsField = secondsLeft % 60
  const minutesField = minutesLeft % 60
  const hoursText = hoursLeft ? `${String(hoursLeft).padStart(2, '0')}:` : ''
  displayText.value = `${hoursText}${String(minutesField).padStart(2, '0')}:${String(secondsField).padStart(2, '0')}`
}

addEventListenerWithLifetime(
  onUnmounted,
  countdownEventTarget,
  'expired',
  (_cd: Countdown) => void update(),
)

const removing = ref(false)
const togglingPause = ref(false)
const restarting = ref(false)

const onRequestRemove = async () => {
  removing.value = true
  const ctx2 = Context.withTimeout(5000, ctx)
  const conn = Connection.getInstance()
  const response = await conn.cancelAlarm(ctx2, countdown.id)
  if (response instanceof Error || response instanceof Abort) {
    console.error('Could not cancel alarm', response)
  }
  removing.value = false
  ctx2.cancel()
}

const onRequestTogglePause = async () => {
  togglingPause.value = true
  try {
    const ctx2 = Context.withTimeout(5000, ctx)
    const conn = Connection.getInstance()
    if (countdown.pausedAtTime) {
      conn.continueAlarm(ctx2, countdown.id)
    } else {
      conn.pauseAlarm(ctx2, countdown.id)
    }
    ctx2.cancel()
  } finally {
    togglingPause.value = false
  }
}

const onRequestRestart = async () => {
  restarting.value = true
  try {
    const ctx2 = Context.withTimeout(5000, ctx)
    const res = await Connection.getInstance().restartAlarm(ctx2, countdown.id)
    ctx2.cancel()
    if (res instanceof Error) {
      alert(res.message)
    }
    if (res === undefined) {
      setTimeout(update, 100)
    }
  } finally {
    restarting.value = false
  }
}

update()

onUnmounted(() => {
  ctx.abort('unmounted')
})

defineExpose({ update })
</script>

<template>
  <div
    style="
      display: flex;
      border: var(--border-width) var(--border-style) var(--color-text-tertiary);
      border-radius: 1em;
      overflow: hidden;
      font-size: var(--font-size-big);
      background-color: var(--color-prim-secondary);
      user-select: none;
      display: flex;
      flex-direction: column;
      align-items: start;
      justify-content: center;
      min-width: 13em;
    "
  >
    <div
      style="
        flex: 1;
        display: flex;
        flex-direction: column;
        padding: 0.4em var(--box-padding-h) 0 var(--box-padding-h);
        justify-content: space-between;
        width: 100%;
        box-sizing: border-box;
      "
    >
      <div style="display: flex; align-items: center; width: 100%">
        <div style="flex: 1"></div>
        <div
          v-if="countdown.name !== undefined"
          style="
            font-size: var(--font-size-small);
            font-weight: bold;
            color: var(--color-text-secondary);
          "
        >
          {{ countdown.name }}
        </div>
        <div
          style="
            flex: 1;
            display: flex;
            align-items: start;
            justify-content: end;
            position: relative;
          "
          :style="{ height: countdown.name === undefined ? 0 : undefined }"
        >
          <IconButton
            :Icon="DetailsIcon"
            @click="dialog?.showModal()"
            title="Show details"
            color="var(--color-text-secondary)"
            style="width: 1em; height: 1em"
            :style="{ position: countdown.name === undefined ? 'absolute' : 'relative' }"
          ></IconButton>
        </div>
      </div>
      <div style="width: 100%; display: flex">
        <div style="flex: 1; display: flex; flex-direction: column; justify-content: center">
          <div
            style="
              width: max-content;
              font-size: var(--font-size-tiny);
              color: var(--color-text-tertiary);
              font-weight: bold;
              margin-right: 1em;
            "
          >
            Time left:
          </div>
        </div>
        <div style="font-size: var(--font-size-bigger)">{{ displayText }}</div>
        <div style="flex: 1"></div>
      </div>
    </div>
    <hr
      style="
        border: var(--border-width) var(--border-style) var(--color-text-tertiary);
        width: 100%;
        box-sizing: border-box;
        margin-top: 0.2em;
        margin-bottom: 0;
      "
    />
    <div
      style="
        display: flex;
        justify-content: center;
        align-items: center;
        width: 100%;
        box-sizing: border-box;
        padding: 0.1em var(--box-padding-h);
      "
      :class="{
        expired: countdown.isExpired,
        paused: countdown.pausedAtTime,
        timer: 1,
      }"
    >
      <div style="flex: 1; display: flex; justify-content: end; align-items: center">
        <IconButton
          :Icon="CancelIcon"
          @click="onRequestRemove"
          title="Remove"
          :disabled="removing"
          style="width: 1em; height: 1em"
        ></IconButton>
      </div>
      <div>
        <IconButton
          v-if="countdown.isExpired"
          :Icon="ReplayIcon"
          @click="onRequestRestart"
          title="Restart"
          style="width: 2em; height: 2em"
        ></IconButton>
        <IconButton
          v-else-if="countdown.pausedAtTime"
          :Icon="PlayArrowIcon"
          @click="onRequestTogglePause"
          title="Continue"
          style="width: 2em; height: 2em"
        ></IconButton>
        <IconButton
          v-else
          :Icon="PauseIcon"
          @click="onRequestTogglePause"
          title="Pause"
          style="width: 2em; height: 2em"
        ></IconButton>
      </div>
      <div style="flex: 1; display: flex; justify-content: start; align-items: center">
        <IconButton
          v-if="!countdown.isExpired"
          :Icon="ReplayIcon"
          @click="onRequestRestart"
          title="Restart"
          style="width: 1em; height: 1em"
        ></IconButton>
      </div>
    </div>
    <dialog
      popover
      closedby="any"
      ref="dialog"
      style="
        background-color: var(--color-secondary);
        padding: 0.4em 0.8em;
        border-style: none;
        border-radius: var(--border-radius-smaller);
        box-shadow: 0px 0px 3px 0px var(--color-primary);
        color: var(--color-text);
      "
    >
      <table
        style="
          background-color: var(--color-secondary);
          border-radius: var(--border-radius-smaller);
          width: max-content;
          font-size: var(--font-size-normal);
          margin-bottom: 1em;
        "
      >
        <tbody>
          <tr v-if="countdown.name">
            <td>Name:</td>
            <td>{{ countdown.name }}</td>
          </tr>
          <tr>
            <td>Started at:</td>
            <td>{{ countdown.startedAt.toLocaleTimeString() }}</td>
          </tr>
          <tr v-if="!countdown.pausedAtTime">
            <td>Expiry at:</td>
            <td>{{ countdown.expiryTimestamp.toLocaleTimeString() }}</td>
          </tr>
          <tr
            v-if="
              countdown.pausedAtTime ||
              countdown.originalExpiryTimestamp.getTime() !== countdown.expiryTimestamp.getTime()
            "
          >
            <td>Initial expiry at:</td>
            <td>{{ countdown.originalExpiryTimestamp.toLocaleTimeString() }}</td>
          </tr>
        </tbody>
      </table>
      <div style="display: flex; justify-content: center">
        <button @click="dialog?.close()" autofocus>Ok</button>
      </div>
    </dialog>
  </div>
</template>

<style lang="css" scoped>
.timer {
  background-color: var(--color-secondary);
}

.timer.expired {
  background-color: var(--color-contrast);
}

.timer.paused {
  background-color: var(--color-timer-paused);
}

td:first-child {
  user-select: none;
  text-align: right;
  color: var(--color-text-secondary);
  font-size: var(--font-size-small);
  font-weight: bold;
}
td:last-child {
  user-select: contain;
}
</style>
