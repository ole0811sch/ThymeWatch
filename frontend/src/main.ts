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
const conn = Connection.new((date, name) =>
  addCountdown({
    expiryTimestamp: date,
    startedAt: new Date(),
    name,
  }),
)

const onExpired = async (countdown: Countdown) => {
  const title = countdown.name ? `Timer expired: ${countdown.name}` : 'Timer expired'
  await audio.start()
  if (Notification.permission === 'granted') {
    const notif = newNotification(title)
    notif.addEventListener('close', (e) => {
      audio.stop()
    })
  } else {
    alert(title)
    audio.stop()
  }
}
countdownEventTarget.addEventListener('expired', onExpired)
