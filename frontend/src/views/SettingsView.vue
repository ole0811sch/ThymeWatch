<script lang="ts" setup>
import { paths } from '@/router/index.ts'

console.log("Settings")
</script>

<template>
  <h1>Settings</h1>
  <RouterLink :to="paths.home">Home</RouterLink>
</template>
