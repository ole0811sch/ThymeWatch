export interface CountdownInput {
  startedAt: Date
  expiryTimestamp: Date
  name?: string
  id: number
  pausedAt?: Date
}
export interface Countdown extends CountdownInput {
  originalExpiryTimestamp: Date
  expired?: boolean
}
export interface CountdownPrivate extends Countdown {
  /** May change */
  timeoutId?: number
}

const countdowns: Map<number, CountdownPrivate> = new Map()

/**
 * @returns copy of countdowns map
 */
export function getCountdowns(): Map<number, Countdown> {
  return new Map(countdowns)
}

export function addCountdown(cd: CountdownInput): Countdown {
  const cdFull: CountdownPrivate = {
    ...cd,
    originalExpiryTimestamp: cd.expiryTimestamp,
  }
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

export function pauseCountdown(id: number): void {
  const cd = countdowns.get(id)
  if (!cd || cd.timeoutId === undefined || cd.expired) {
    return
  }

  cd.pausedAt = new Date()
  handler('paused', cd)
}

export function continueCountdown(id: number): void {
  const cd = countdowns.get(id)
  if (!cd || cd.timeoutId !== undefined || cd.pausedAt === undefined || cd.expired) {
    return
  }

  cd.expiryTimestamp = new Date(
    Math.min(
      cd.expiryTimestamp.getTime() + Math.max(Date.now() - cd.pausedAt.getTime(), 0),
      8_640_000_000_000_000,
    ),
  )
  cd.pausedAt = undefined
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
