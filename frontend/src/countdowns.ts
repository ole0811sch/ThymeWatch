export interface CountdownInput {
  startedAt: Date
  expiryTimestamp: Date
  name?: string
  id?: string
  pausedAt?: Date
}
export interface Countdown extends CountdownInput {
  id: string
  originalExpiryTimestamp: Date
  expired?: boolean
}
export interface CountdownPrivate extends Countdown {
  /** May change */
  timeoutId?: number
}

const countdowns: Map<string, CountdownPrivate> = new Map()

/**
 * @returns copy of countdowns map
 */
export function getCountdowns(): Map<string, Countdown> {
  return new Map(countdowns)
}

/**
 * Calls `callback` at some point "soon" after `until`, and stores the id of the
 * setTimeout id in `outId[0]`. This id may change as multiple calls to
 * setTimeout may be necessary.
 */
function longTimeout(callback: () => void, until: Date, setId: (id: number) => void): void {
  setId(
    setTimeout(
      () => {
        if (until.getTime() <= Date.now()) {
          callback()
        } else {
          longTimeout(callback, until, setId)
        }
      },
      Math.min(2147483647, until.getTime() - Date.now()),
    ),
  )
}

/**
 * Sets timeout that expires at cd.expiryTimestamp and emits the 'expired'
 * event, and sets cd.timeoutId
 */
function startTimeout(cd: CountdownPrivate) {
  longTimeout(
    () => {
      cd.expired = true
      handler('expired', cd)
    },
    cd.expiryTimestamp,
    (id) => (cd.timeoutId = id),
  )
}

/**
 * Clears timeout associated with cd.timeoutId and sets the field to undefined
 */
function stopTimeout(cd: CountdownPrivate) {
  clearTimeout(cd.timeoutId)
  cd.timeoutId = undefined
}

export function addCountdown(cd: CountdownInput): Countdown {
  const cdFull: CountdownPrivate = {
    ...cd,
    id: crypto.randomUUID(),
    originalExpiryTimestamp: cd.expiryTimestamp,
  }
  countdowns.set(cdFull.id, cdFull)
  startTimeout(cdFull)
  handler('new', cdFull)
  return cdFull
}

export function removeCountdown(id: string): void {
  const cd = countdowns.get(id)
  if (!cd) {
    return
  }

  stopTimeout(cd)
  countdowns.delete(id)
  handler('removed', cd)
  for (const type of countdownEventTypes) {
    handlers[type].delete(id)
  }
}

export function pauseCountdown(id: string): void {
  const cd = countdowns.get(id)
  if (!cd || cd.timeoutId === undefined || cd.expired) {
    return
  }

  cd.pausedAt = new Date()
  stopTimeout(cd)
  handler('paused', cd)
}

export function continueCountdown(id: string): void {
  const cd = countdowns.get(id)
  if (!cd || cd.timeoutId !== undefined || cd.pausedAt === undefined || cd.expired) {
    return
  }

  cd.expiryTimestamp = new Date(
    cd.expiryTimestamp.getTime() + Math.max(Date.now() - cd.pausedAt.getTime(), 0),
  )
  cd.pausedAt = undefined
  startTimeout(cd)
  handler('continued', cd)
}

function handler(type: CountdownEventType, cd: Countdown): void {
  handlers[type].get(cd.id)?.forEach((handler) => handler(cd))
  handlers[type].get(null)?.forEach((handler) => handler(cd))
}

const countdownEventTypes = ['expired', 'new', 'removed', 'paused', 'continued'] as const
const handlers: Record<
  CountdownEventType,
  Map<string | null, Set<(cd: Countdown) => void>>
> = Object.fromEntries(countdownEventTypes.map((type) => [type, new Map()])) as any
type CountdownEventType = (typeof countdownEventTypes)[number]
export const countdownEventTarget = {
  addEventListener(
    type: CountdownEventType,
    listener: (cd: Countdown) => void,
    { countdownId = null }: { countdownId?: string | null } = {},
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
    { countdownId = null }: { countdownId?: string | null } = {},
  ): void {
    handlers[type].get(countdownId)?.delete(callback)
  },
}
