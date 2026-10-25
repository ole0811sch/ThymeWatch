import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import {
  addCountdown,
  continueCountdown,
  countdownEventTarget,
  expireCountdown,
  pauseCountdown,
  removeAllCountdown,
  removeCountdown,
  type Countdown,
} from './countdowns.ts'
import { audio } from './audio.ts'
import { closeNotification, newNotification, notifs } from './notifs.ts'
import { Connection, type ConnectionEventMap } from './connection.ts'
import { Abort, Context } from './context.ts'

const app = createApp(App)

app.use(router)

app.mount('#app')

Notification.requestPermission()
const ctx = Context.background()
const conn = Connection.getInstance()
async function trySubscribe() {
  for (let i = 0; i < 10; ++i) {
    const res = await conn.subscribe(ctx)
    if (!(res instanceof Abort || res instanceof Error)) {
      return
    }
  }
  alert('Could not connect to server')
}
if (conn.state === 'open') {
  await trySubscribe()
}
conn.addEventListener('reconnected', trySubscribe)
conn.addEventListener('ketchup', () => void removeAllCountdown())
conn.addEventListener(
  'setalarm',
  ({
    expiryTime,
    name,
    id,
    startedAt,
    originalExpiryTime,
    pausedAtTime,
    isExpired,
    isDismissed,
  }) => {
    addCountdown({
      expiryTimestamp: expiryTime,
      startedAt,
      name,
      id,
      originalExpiryTimestamp: originalExpiryTime,
      pausedAtTime,
      isExpired: isExpired,
      isDismissed: isDismissed,
    })
  },
)
conn.addEventListener('cancelalarm', (alarmId) => {
  closeNotification(alarmId)
  removeCountdown(alarmId)
})
conn.addEventListener('alarmpaused', ({ id, pausedAtTime }) => {
  pauseCountdown(id, pausedAtTime)
})
conn.addEventListener('alarmcontinued', ({ id, newExpiryTimestamp }) => {
  continueCountdown(id, newExpiryTimestamp)
})
let retryCount = 0
const MAX_RETRY_COUNT = 6
const retryCallbackId: [null | number] = [null]
const tryReconnected = () => {
  async function f() {
    if (retryCount >= MAX_RETRY_COUNT || conn.state === 'open') {
      return
    }
    ++retryCount
    await conn.reconnect()
    //@ts-expect-error conn.state may be affected by conn.reconnect()
    if (conn.state !== 'open') {
      retryCallbackId[0] = setTimeout(f, 10 * 1000)
    }
  }
  if (retryCallbackId[0] === null) {
    f()
  }
}
conn.addEventListener('closed', (e) => {
  if (!e.weClosed) {
    console.error(`Server closed connection (${e.code}): ${e.reason}`)
  }
  tryReconnected()
})
conn.addEventListener('reconnected', () => {
  retryCount = 0
  if (retryCallbackId[0] !== null) {
    clearTimeout(retryCallbackId[0])
    retryCallbackId[0] = null
  }
})
if (conn.state === 'closed') {
  tryReconnected()
}

const onExpired = async (countdown: Countdown) => {
  if (countdown.isDismissed) {
    return
  }
  const title = countdown.name ? `Timer expired: ${countdown.name}` : 'Timer expired'
  await audio.start()
  if (Notification.permission === 'granted') {
    const notif = newNotification(countdown, title, {
      requireInteraction: true,
    })
    for (const type of ['close', 'click']) {
      notif.addEventListener(type, (e) => {
        audio.stop()
      })
    }
  } else {
    alert(title)
    audio.stop()
  }
}
countdownEventTarget.addEventListener('expired', onExpired)
conn.addEventListener('expirealarm', (id) => {
  expireCountdown(id)
})
conn.addEventListener('alarmsdismissed', (ids) => {
  for (const id of ids) {
    closeNotification(id)
  }
})
