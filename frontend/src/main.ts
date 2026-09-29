import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { Connection } from './test.ts'
import { addCountdown, countdownEventTarget, type Countdown } from './countdowns.ts'

const app = createApp(App)

app.use(router)

app.mount('#app')

Notification.requestPermission()
const conn = Connection.new((date) =>
  addCountdown({
    expiryTimestamp: date,
    startedAt: new Date(),
  }),
)

const onExpired = async (countdown: Countdown) => {
  const title = countdown.name ? `Timer expired: ${countdown.name}` : 'Timer expired'
  if (Notification.permission === 'granted') {
    new Notification(title)
  } else {
    alert(title)
  }
}
countdownEventTarget.addEventListener('expired', onExpired)
