/**
 * When manually closing notifications, always first remove it from notifs and
 * then close it. That way onclose handlers can check if they need to do something.
 */
const notifs: Set<Notification> = new Set()

export function newNotification(...args: ConstructorParameters<typeof Notification>): Notification {
  const notif = new Notification(...args)
  notifs.add(notif)
  return notif
}

export function closeAllNotifications() {
  const notifsCpy = [...notifs]
  notifs.clear()
  notifsCpy.forEach((n) => n.close())
}
