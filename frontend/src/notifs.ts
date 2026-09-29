import { ref, type Ref } from 'vue'
import type { Countdown } from './countdowns'

/**
 * When manually closing notifications, always first remove it from notifs and
 * then close it. That way onclose handlers can check if they need to do something.
 */
export const notifs: Ref<Map<string, { notif: Notification; countdown: Countdown }>> = ref(
  new Map(),
)

export function newNotification(
  countdown: Countdown,
  ...args: ConstructorParameters<typeof Notification>
): Notification {
  const notif = new Notification(...args)
  notifs.value.set(countdown.id, { notif, countdown })
  notif.addEventListener('close', () => notifs.value.delete(countdown.id))
  return notif
}

export function closeAllNotifications() {
  const notifsCpy = [...notifs.value]
  notifs.value.clear()
  notifsCpy.forEach(([_, { notif }]) => notif.close())
}
