export interface CountdownInput {
  startedAt: Date
  expiryTimestamp: Date
  name?: string
  id?: string
}
export interface Countdown extends CountdownInput {
  id: string
  abortController: AbortController
}

const countdowns: Map<string, Countdown> = new Map()

/**
 * @returns copy of countdowns map
 */
export function getCountdowns(): Map<string, Countdown> {
  return new Map(countdowns)
}

export function addCountdown(cd: CountdownInput): Countdown {
  const cdFull: Countdown = {
    ...cd,
    id: crypto.randomUUID(),
    abortController: new AbortController(),
  }
  cdFull.abortController.signal.addEventListener('abort', () => {
    clearTimeout(timeoutId)
    countdowns.delete(cdFull.id)
    handler('cancelled', cdFull)
    for (const type of countdownEventTypes) {
      handlers[type].delete(cdFull.id)
    }
  })
  countdowns.set(cdFull.id, cdFull)
  const timeoutId = setTimeout(
    () => handler('expired', cdFull),
    cdFull.expiryTimestamp.getTime() - Date.now(),
  )
  handler('new', cdFull)
  return cdFull
}

function handler(type: CountdownEventType, cd: Countdown): void {
  handlers[type].get(cd.id)?.forEach((handler) => handler(cd))
  handlers[type].get(null)?.forEach((handler) => handler(cd))
}

const handlers: Record<CountdownEventType, Map<string | null, Set<(cd: Countdown) => void>>> = {
  new: new Map(),
  cancelled: new Map(),
  expired: new Map(),
}
const countdownEventTypes = ['expired', 'new', 'cancelled'] as const
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
