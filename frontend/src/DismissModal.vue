<script setup lang="ts">
import { closeAllNotifications, notifs } from './notifs'
import { audio } from './audio'
</script>

<template>
  <div
    style="
      position: absolute;
      left: 0;
      top: 0;
      right: 0;
      bottom: 0;
      background-color: hsl(from var(--color-text) h s l / 0.3);
      display: flex;
      align-items: center;
      justify-content: center;
    "
  >
    <div
      style="
        position: absolute;
        margin-left: auto;
        margin-right: auto;
        background-color: var(--color-primary);
        width: 40em;
        height: 20em;
        padding: 0 1em;
        border-style: none;
        border-radius: var(--border-radius-small);
        box-shadow: 0px 0px 3px 0px var(--color-primary);
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
            audio.stop()
            closeAllNotifications()
          }
        "
      >
        Dismiss
      </button>
    </div>
  </div>
</template>
