import AlarmsView from '@/views/AlarmsView.vue'
import SettingsView from '@/views/SettingsView.vue'
import { createRouter, createWebHashHistory, createWebHistory } from 'vue-router'

export const paths = {
  home: '/',
  settings: '/settings',
} as const
const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    { path: paths.home, component: AlarmsView },
    { path: paths.settings, component: SettingsView },
  ],
})

export default router
