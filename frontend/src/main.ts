import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import {
  addCountdown,
  continueCountdown,
  dismissCountdown,
  expireCountdown,
  pauseCountdown,
  removeAllCountdown,
  removeCountdown,
} from './countdowns.ts'
import { Connection } from './connection.ts'
import { Abort, Context } from './utils/context.ts'

const app = createApp(App)

app.use(router)

app.mount('#app')

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
    replacingId,
    expiryTime,
    name,
    id,
    originalId,
    startedAt,
    originalExpiryTime,
    pausedAtTime,
    isExpired,
    isDismissed,
  }) => {
    addCountdown(
      {
        expiryTimestamp: expiryTime,
        startedAt,
        name,
        id,
        originalId,
        originalExpiryTimestamp: originalExpiryTime,
        pausedAtTime,
        isExpired: isExpired,
        isDismissed: isDismissed,
      },
      replacingId ?? undefined,
    )
  },
)
conn.addEventListener('cancelalarm', (alarmId) => {
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

conn.addEventListener('expirealarm', (id) => {
  expireCountdown(id)
})
conn.addEventListener('alarmsdismissed', (ids) => {
  for (const id of ids) {
    dismissCountdown(id)
  }
})
