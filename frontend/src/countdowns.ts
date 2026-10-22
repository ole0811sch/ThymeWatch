export interface CountdownInput {
  startedAt: Date
  expiryTimestamp: Date
  originalExpiryTimestamp: Date
  name?: string
  id: number
  pausedAtTime: Date | null
}
export interface Countdown extends CountdownInput {
  expired?: boolean
}

const countdowns: Map<number, Countdown> = new Map()

/**
 * @returns copy of countdowns map
 */
export function getCountdowns(): Map<number, Countdown> {
  return new Map(countdowns)
}

export function addCountdown(cd: CountdownInput): Countdown {
  const cdFull: Countdown = { ...cd }
  countdowns.set(cdFull.id, cdFull)
  handler('new', cdFull)
  return cdFull
}

export function removeCountdown(id: number): void {
  const cd = countdowns.get(id)
  if (!cd) {
    return
  }

  countdowns.delete(id)
  handler('removed', cd)
  for (const type of countdownEventTypes) {
    handlers[type].delete(id)
  }
}

export function removeAllCountdown(): void {
  const cpy = new Map(countdowns)
  countdowns.clear()
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
  const cd = countdowns.get(id)
  if (!cd || cd.expired) {
    return
  }

  cd.pausedAtTime = pausedAtTime
  handler('paused', cd)
}

export function continueCountdown(id: number, newExpiryTime: Date): void {
  const cd = countdowns.get(id)
  if (!cd || cd.expired) {
    return
  }

  cd.expiryTimestamp = newExpiryTime
  cd.pausedAtTime = null
  handler('continued', cd)
}

function handler(type: CountdownEventType, cd: Countdown): void {
  handlers[type].get(cd.id)?.forEach((handler) => handler(cd))
  handlers[type].get(null)?.forEach((handler) => handler(cd))
}

const countdownEventTypes = ['expired', 'new', 'removed', 'paused', 'continued'] as const
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
