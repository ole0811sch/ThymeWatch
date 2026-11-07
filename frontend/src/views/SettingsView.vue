<script lang="ts" setup>
import { Connection } from '@/connection';
import { Abort, Context } from '@/utils/context';
import { paths } from '@/router/index.ts'

const kill = async () => {
  const err = await Connection.getInstance().kill(Context.background<never>())
  if (err instanceof Error) {
    alert(err.message)
  } else if (err instanceof Abort) {
    alert("Could not stop server: timeout")
  }
}
</script>

<template>
  <h1 style="margin-top: 0; margin-left: auto; margin-right: auto;">Settings</h1>
  <div class="settings-list">
    <RouterLink :to="paths.home" style="display: block flow">Home</RouterLink>
    <button @click="kill" style="width: fit-content">Stop server</button>
  </div>
</template>

<style lang="css" scoped>
.settings-list > * {
  margin-top: 0.5em;
}
</style>
