import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { addCountdown, countdownEventTarget, type Countdown } from './countdowns.ts'
import { audio } from './audio.ts'
import { newNotification } from './notifs.ts'

const app = createApp(App)

app.use(router)

app.mount('#app')

Notification.requestPermission()
if (window.electronApi) {
  window.electronApi.onSetAlarm(async (alarmConfig) => {
    addCountdown({
      expiryTimestamp: alarmConfig.expiryTime,
      startedAt: new Date(),
      name: alarmConfig.name,
    })
  })
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
