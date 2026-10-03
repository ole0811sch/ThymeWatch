import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { Connection } from './test.ts'
import { addCountdown, countdownEventTarget, type Countdown } from './countdowns.ts'
import { audio } from './audio.ts'
import { newNotification } from './notifs.ts'

const app = createApp(App)

app.use(router)

app.mount('#app')

Notification.requestPermission()
const conn = await Connection.getInstance()
conn.addEventListener('setalarm', ([date, name]) =>
  addCountdown({
    expiryTimestamp: date,
    startedAt: new Date(),
    name,
  }),
)
let retryCount = 0
const MAX_RETRY_COUNT = 120
const retryCallbackId: [null | number] = [null]
const onClosed = () => {
  async function f() {
    console.log({ retryCount })
    if (retryCount >= MAX_RETRY_COUNT || conn.state === 'open') {
      return
    }
    ++retryCount
    await conn.reconnect()
    //@ts-expect-error conn.state may be affected by conn.reconnect()
    if (conn.state !== 'open') {
      retryCallbackId[0] = setTimeout(f, 1000)
    }
  }
  if (retryCallbackId[0] === null) {
    f()
  }
}
conn.addEventListener('closed', onClosed)
conn.addEventListener('reconnected', () => {
  retryCount = 0
  if (retryCallbackId[0] !== null) {
    clearTimeout(retryCallbackId[0])
    retryCallbackId[0] = null
  }
})
if (conn.state === 'closed') {
  onClosed()
}

const onExpired = async (countdown: Countdown) => {
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
