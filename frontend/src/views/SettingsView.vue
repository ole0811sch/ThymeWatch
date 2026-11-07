<script lang="ts" setup>
import { Connection } from '@/connection';
import { Abort, Context } from '@/utils/context';
import { paths } from '@/router/index.ts'
import TitleBar from '@/components/TitleBar.vue';

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
  <TitleBar title="Settings" back-path="home"></TitleBar>
  <div class="page-container settings-list">
    <button @click="kill" style="width: fit-content">Stop server</button>
  </div>
</template>

<style lang="css" scoped>
.settings-list > * {
  margin-top: 0.5em;
}
</style>
