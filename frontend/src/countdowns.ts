import { ref, type Ref } from 'vue'

export interface Countdown {
  startedAt: Date
  expiryTimestamp: Date
  originalExpiryTimestamp: Date
  name?: string
  id: number
  originalId: number
  pausedAtTime: Date | null
  isExpired: boolean
  isDismissed: boolean
}

export const countdowns: Ref<Map<number, Countdown>> = ref(new Map())
export const countdownsByOrigId: Ref<Map<number, Countdown>> = ref(new Map())
export const expiredUndismissedAlarms = ref(new Set<number>())

export function addCountdown(cd: Countdown, replacingId?: number): Countdown {
  if (replacingId !== undefined) {
    dismissCountdown(replacingId)
    countdowns.value.delete(replacingId)
  }
  const cdFull: Countdown = { ...cd }
  countdowns.value.set(cdFull.id, cdFull)
  countdownsByOrigId.value.set(cdFull.originalId, cdFull)
  handler('new', cdFull)
  if (cdFull.isExpired && !cdFull.isDismissed) {
    expiredUndismissedAlarms.value.add(cd.id)
  }
  if (cdFull.isExpired) {
    handler('expired', cdFull)
  }
  if (cdFull.isDismissed) {
    handler('dismissed', cdFull)
  }
  return cdFull
}

export function removeCountdown(id: number): void {
  dismissCountdown(id)
  const cd = countdowns.value.get(id)
  if (!cd) {
    return
  }

  countdowns.value.delete(id)
  countdownsByOrigId.value.delete(cd.originalId)
  handler('removed', cd)
  for (const type of countdownEventTypes) {
    handlers[type].delete(id)
  }
}

export function removeAllCountdown(): void {
  const cpy = new Map(countdowns.value)
  countdowns.value.clear()
  countdownsByOrigId.value.clear()
  for (const cd of cpy.values()) {
    handler('removed', cd)
  }
  for (const type of countdownEventTypes) {
    const nullEntry = handlers[type].get(null)
    handlers[type].clear()
    if (nullEntry) {
      handlers[type].set(null, nullEntry)
    }
  }
}

export function pauseCountdown(id: number, pausedAtTime: Date): void {
  const cd = countdowns.value.get(id)
  if (!cd || cd.isExpired) {
    return
  }

  cd.pausedAtTime = pausedAtTime
  handler('paused', cd)
}

export function continueCountdown(id: number, newExpiryTime: Date): void {
  const cd = countdowns.value.get(id)
  if (!cd || cd.isExpired) {
    return
  }

  cd.expiryTimestamp = newExpiryTime
  cd.pausedAtTime = null
  handler('continued', cd)
}

export function expireCountdown(id: number): void {
  const cd = countdowns.value.get(id)
  if (!cd || cd.isExpired) {
    return
  }

  cd.isExpired = true
  if (!cd.isDismissed) {
    expiredUndismissedAlarms.value.add(id)
  }
  handler('expired', cd)
}

export function dismissCountdown(id: number): void {
  const cd = countdowns.value.get(id)
  if (!cd || cd.isDismissed || !cd.isExpired) {
    return
  }

  cd.isDismissed = true
  expiredUndismissedAlarms.value.delete(id)
  handler('dismissed', cd)
}

function handler(type: CountdownEventType, cd: Countdown): void {
  handlers[type].get(cd.id)?.forEach((handler) => handler(cd))
  handlers[type].get(null)?.forEach((handler) => handler(cd))
}

const countdownEventTypes = [
  'expired',
  'new',
  'removed',
  'paused',
  'continued',
  'dismissed',
] as const
const handlers: Record<
  CountdownEventType,
  Map<number | null, Set<(cd: Countdown) => void>>
> = Object.fromEntries(countdownEventTypes.map((type) => [type, new Map()])) as any
type CountdownEventType = (typeof countdownEventTypes)[number]
export const countdownEventTarget = {
  addEventListener(
    type: CountdownEventType,
    listener: (cd: Countdown) => void,
    { countdownId = null }: { countdownId?: number | null } = {},
  ): void {
    let stored = handlers[type].get(countdownId)
    if (!stored) {
      stored = new Set()
      handlers[type].set(countdownId, stored)
    }
    stored.add(listener)
  },
  removeEventListener(
    type: CountdownEventType,
    callback: (cd: Countdown) => void,
    { countdownId = null }: { countdownId?: number | null } = {},
  ): void {
    handlers[type].get(countdownId)?.delete(callback)
  },
}
