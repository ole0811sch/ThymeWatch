import { ref } from 'vue'

let isInited = false
let audioEl: HTMLMediaElement

export const isPlaying = ref(false)

function init() {
  if (isInited) {
    return
  }
  const el = document.getElementById('audio')
  if (!(el instanceof HTMLMediaElement)) {
    return
  }
  el.volume = 0.4
  isInited = true
  audioEl = el
}

export const audio = {
  async start(this: void): Promise<undefined | DOMException> {
    init()
    if (audioEl.paused) {
      try {
        await audioEl.play()
      } catch (e) {
        if (e instanceof DOMException) {
          return e
        }
      }
      isPlaying.value = true
    }
  },

  stop(this: void) {
    init()
    audioEl.pause()
    audioEl.currentTime = 0
    isPlaying.value = false
  },
}
