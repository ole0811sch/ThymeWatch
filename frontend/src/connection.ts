import { Abort, Context, type TimeoutContext } from './utils/context'
import type { IntersectingUnionIndex } from './utils/types'

export const tags = [
  'CREATE_ALARM',
  'RESTART_ALARM',
  'CANCEL_ALARM',
  'DISMISS_ALARMS',
  'PAUSE_ALARM',
  'CONTINUE_ALARM',
  'SUBSCRIBE',
  'NOTIFY_ALARM',
  'ALARM_CANCELED',
  'ALARM_PAUSED',
  'ALARM_CONTINUED',
  'ALARM_EXPIRED',
  'ALARMS_DISMISSED',
  'KILL',
] as const
export type Tag = (typeof tags)[number]
export type MessageBody = {
  [x: string]: unknown
}
export type MessageHeader = {
  id: number
} & (
  | {
      inReplyTo: number
      tag?: null
    }
  | {
      tag: string
      inReplyTo?: null
    }
)
const connectionEventTypes = [
  'reconnected',
  'reconnecting',
  'closed',
  'setalarm',
  'cancelalarm',
  'expirealarm',
  'alarmsdismissed',
  'alarmpaused',
  'alarmcontinued',
  'ketchup',
] as const
/**
 * ketchup is fired when the server resends all alarms after reconnect. It's
 * fired before any of the alarms' events are fired
 */
export type ConnectionEventType = (typeof connectionEventTypes)[number]

export type ConnectionEventMap = {
  reconnecting: undefined
  reconnected: undefined
  closed: {
    /** whether the closing was requested by our end of the connection */
    weClosed: boolean
    reason: string
    code: number
  }
  setalarm: {
    replacingId: number | null
    expiryTime: Date
    name: string | undefined
    id: number
    originalId: number
    startedAt: Date
    ketchup: boolean
    pausedAtTime: Date | null
    originalExpiryTime: Date
    isExpired: boolean
    isDismissed: boolean
  }
  cancelalarm: number
  expirealarm: number
  alarmsdismissed: number[]
  ketchup: undefined
  alarmpaused: {
    pausedAtTime: Date
    id: number
  }
  alarmcontinued: {
    newExpiryTimestamp: Date
    id: number
  }
}

function extractOptionalValue<T extends object, R>(
  v: T,
  attr: string,
  pred: (u: unknown) => u is R,
): [value: R | undefined | null, ok: boolean] {
  if (!(attr in v)) {
    return [undefined, true]
  }
  const field: unknown = v[attr as keyof typeof v]
  if (field === null || field === undefined || pred(field)) {
    return [field, true]
  }
  return [undefined, false]
}

function verifyNotifyAlarmMessage(
  notif: unknown,
  ketchup: boolean,
): ConnectionEventMap['setalarm'] | undefined {
  if (notif === null || typeof notif !== 'object') {
    return
  }
  const [replacingId, replacingIdOk] = extractOptionalValue(
    notif,
    'replacingId',
    (u) => typeof u === 'number',
  )
  if (!replacingIdOk) {
    return
  }
  if (!('alarm' in notif) || notif.alarm === null || typeof notif.alarm !== 'object') {
    return
  }
  const alarm = notif.alarm
  if (
    !(
      'id' in alarm &&
      'originalId' in alarm &&
      'spec' in alarm &&
      'isExpired' in alarm &&
      'isDismissed' in alarm &&
      'spec' in alarm &&
      'originalExpiryTime' in alarm
    ) ||
    typeof alarm.id !== 'number' ||
    typeof alarm.originalId !== 'number' ||
    typeof alarm.spec !== 'object' ||
    typeof alarm.isExpired !== 'boolean' ||
    typeof alarm.isDismissed !== 'boolean' ||
    alarm.spec === null ||
    typeof alarm.spec !== 'object' ||
    typeof alarm.originalExpiryTime !== 'string'
  ) {
    return
  }

  const [pausedAtTimeStr, pausedAtTimeStrOk] = extractOptionalValue(
    alarm,
    'pausedAtTime',
    (u) => typeof u === 'string',
  )
  if (!pausedAtTimeStrOk) {
    return
  }
  const spec = alarm.spec
  const [name, nameStrOk] = extractOptionalValue(alarm, 'name', (u) => typeof u === 'string')
  if (!nameStrOk) {
    return
  }
  if (
    !('expiryTime' in spec && 'startedAt' in spec) ||
    typeof spec.expiryTime !== 'string' ||
    typeof spec.startedAt !== 'string'
  ) {
    return
  }
  const expiryDate = new Date(spec.expiryTime)
  const startedAt = new Date(spec.startedAt)
  const originalExpiryTime = new Date(alarm.originalExpiryTime)
  const pausedAtTime = typeof pausedAtTimeStr === 'string' ? new Date(pausedAtTimeStr) : null
  if (
    isNaN(expiryDate.getTime()) ||
    isNaN(startedAt.getTime()) ||
    isNaN(originalExpiryTime.getTime()) ||
    isNaN(pausedAtTime?.getTime() ?? 0)
  ) {
    return
  }

  return {
    replacingId: replacingId ?? null,
    expiryTime: expiryDate,
    name: name ?? undefined,
    id: alarm.id,
    originalId: alarm.originalId,
    startedAt: startedAt,
    ketchup,
    isExpired: alarm.isExpired,
    isDismissed: alarm.isDismissed,
    originalExpiryTime: originalExpiryTime,
    pausedAtTime: pausedAtTime,
  }
}

export const weClosedSym = Symbol('weClosedSym')

export interface MyWebSocket extends WebSocket {
  [weClosedSym]?: boolean
}

export class Connection {
  private i = 0
  private static connectedInstance: Promise<Connection | Error> | null = null
  private static instance: Connection | null = null
  private messageTimeoutMs: number = 1000
  private reconnecting: Promise<undefined> | null = null
  private handlers: {
    [T in ConnectionEventType]: Set<(data: ConnectionEventMap[T]) => void>
  } = Object.fromEntries(connectionEventTypes.map((name) => [name, new Set()] as const)) as any
  private constructor(
    private socket: MyWebSocket | null,
    private promises: Map<number, (response: [MessageHeader, MessageBody] | Error) => void>,
  ) {}

  async sendMessage<T>(
    ctx: Context<T>,
    tagOrReplyId: Tag | number,
    obj: MessageBody,
  ): Promise<number | Error | Abort<T>> {
    while (this.socket === null) {
      const res = await Promise.race([this.reconnect(), ctx.reason])
      if (res instanceof Error || res instanceof Abort) {
        return res
      }
    }
    const id = this.i++
    this.socket.send(
      JSON.stringify({
        id,
        ...(typeof tagOrReplyId === 'string' ? { tag: tagOrReplyId } : { inReplyTo: tagOrReplyId }),
      }) +
        '\n' +
        JSON.stringify(obj),
    )
    return id
  }

  async asyncSendMessage<T>(
    ctx: Context<T>,
    tagOrReplyId: Tag | number,
    obj: MessageBody,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<T>> {
    const id = await this.sendMessage(ctx, tagOrReplyId, obj)
    if (id instanceof Error || id instanceof Abort) {
      return id
    }
    let res: (response: [MessageHeader, MessageBody] | Error) => void
    const prom = new Promise<[MessageHeader, MessageBody] | Error>((res_) => void (res = res_))
    this.promises.set(id, res!)
    const winner = await Promise.race([prom, ctx.reason])
    if (winner instanceof Abort) {
      this.promises.delete(id)
    }
    return winner
  }

  close() {
    if (!this.socket) {
      return
    }
    this.socket.close()
  }

  private async handleMessage<T>(
    ctx: Context<T>,
    event: MessageEvent<string>,
  ): Promise<undefined | Abort<T>> {
    console.log('Message from server: ', event.data)
    const data = event.data
    const firstSpaceIdx = data.indexOf('\n')
    if (firstSpaceIdx === -1) {
      console.error('Invalid message syntax in message from server: ' + data)
      return
    }
    const parts = [data.slice(0, firstSpaceIdx), data.slice(firstSpaceIdx + 1)] as const
    let headerUnvalidated: Record<string, unknown>
    try {
      headerUnvalidated = JSON.parse(parts[0])
    } catch (e) {
      console.error('Invalid message json in message header from server: ' + parts[0])
      return
    }
    headerUnvalidated.inReplyTo ??= null
    headerUnvalidated.tag ??= null
    if (
      typeof headerUnvalidated.id !== 'number' ||
      !(headerUnvalidated.inReplyTo === null || typeof headerUnvalidated.inReplyTo === 'number') ||
      !(
        headerUnvalidated.tag === null ||
        (tags as readonly unknown[]).includes(headerUnvalidated.tag)
      ) ||
      (headerUnvalidated.inReplyTo === null && headerUnvalidated.tag === null)
    ) {
      console.error('Invalid header from server: ' + parts[0])
      return
    }
    const header = headerUnvalidated as MessageHeader

    let body: MessageBody
    try {
      body = JSON.parse(parts[1])
    } catch (e) {
      console.error('Invalid message json in message body from server: ' + parts[1])
      return
    }

    const tuple: [MessageHeader, MessageBody] = [header, body]
    if (header.inReplyTo != null) {
      const prom = this.promises.get(header.inReplyTo)
      if (prom === undefined) {
        console.error('Received message with invalid replying-to id ' + headerUnvalidated.inReplyTo)
        return
      }
      prom(tuple)
      this.promises.delete(header.inReplyTo)
      return
    }

    let eventName
    switch (header.tag) {
      case 'NOTIFY_ALARM': {
        let events
        if (typeof body.ketchup === 'boolean' && Array.isArray(body.alarms)) {
          const eventsUnfiltered = body.alarms.map((alarm) =>
            verifyNotifyAlarmMessage(alarm, !!body.ketchup),
          )
          if (eventsUnfiltered.every((event) => event !== undefined)) {
            events = eventsUnfiltered
          }
        }
        if (events === undefined) {
          console.error('Invalid payload for NOTIFY_ALARM')
          break
        }
        if (body.ketchup) {
          this.emit('ketchup', undefined)
        }
        for (const event of events) {
          this.emit('setalarm', event)
        }
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      case 'ALARM_CANCELED':
        eventName ??= 'cancelalarm' as const
      case 'ALARM_EXPIRED': {
        eventName ??= 'expirealarm' as const
        if (typeof body.id !== 'number') {
          console.error('Invalid type of field `id`')
          break
        }
        this.emit(eventName, body.id)
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      case 'ALARMS_DISMISSED': {
        if (!Array.isArray(body.ids) || body.ids.some((id) => typeof id !== 'number')) {
          console.error('Invalid type of field `id`')
          break
        }
        this.emit('alarmsdismissed', body.ids)
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      case 'ALARM_CONTINUED': {
        eventName ??= 'alarmcontinued' as const
        let newExpiryTime: Date
        if (
          typeof body.id !== 'number' ||
          typeof body.newExpiryTime !== 'string' ||
          ((newExpiryTime = new Date(body.newExpiryTime)), isNaN(newExpiryTime.getTime()))
        ) {
          console.error('Invalid payload for ALARM_CONTINUED')
          break
        }
        this.emit('alarmcontinued', {
          id: body.id,
          newExpiryTimestamp: newExpiryTime,
        })
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      case 'ALARM_PAUSED': {
        let pausedAtTime: Date
        if (
          typeof body.id !== 'number' ||
          typeof body.pausedAtTime !== 'string' ||
          ((pausedAtTime = new Date(body.pausedAtTime)), isNaN(pausedAtTime.getTime()))
        ) {
          console.error('Invalid payload for ALARM_PAUSED')
          break
        }
        this.emit('alarmpaused', {
          id: body.id,
          pausedAtTime: pausedAtTime,
        })
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      default:
        console.log(tuple)
    }
  }

  private makeMessageContext<T, U = never>(ctx: Context<T>): Context<T | U | 'timeout'> {
    const ctx2 = Context.withTimeout<T, U>(this.messageTimeoutMs, ctx)
    ctx2.onTimeout(() => {
      console.error('Message timed out')
    })
    return ctx2
  }

  private async asyncSendMessageDefaultTimeout<T>(
    ctx: Context<T>,
    tagOrReplyId: Tag | number,
    obj: MessageBody,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<T | 'timeout'>> {
    const ctx2 = this.makeMessageContext(ctx)
    const ret = await this.asyncSendMessage(ctx2, tagOrReplyId, obj)
    ctx2.cancel()
    return ret
  }

  async createAlarm<T>(
    ctx: Context<T>,
    expiryTime: Date,
    startedAt: Date,
    name?: string,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'CREATE_ALARM', {
      name: name,
      expiryTime: expiryTime,
      startedAt: startedAt,
    })
  }

  async restartAlarm<T>(
    ctx: Context<T>,
    id: number,
  ): Promise<undefined | Error | Abort<'timeout' | T>> {
    const response = await this.asyncSendMessageDefaultTimeout(ctx, 'RESTART_ALARM', { id })
    if (!Array.isArray(response)) {
      return response
    }
    switch (response[1].status) {
      case 200:
        return
      case 404:
        return Error("Couldn't restart alarm: Alarm doesn't exist anymore")
      default:
        console.error({ status: response[1].status }, 'Unsupported status')
        return Error('Error while restarting alarm: Server Error')
    }
  }

  async cancelAlarm<T>(
    ctx: Context<T>,
    id: number,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'CANCEL_ALARM', {
      id: id,
    })
  }

  async pauseAlarm<T>(
    ctx: Context<T>,
    id: number,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'PAUSE_ALARM', {
      id: id,
    })
  }

  async continueAlarm<T>(
    ctx: Context<T>,
    id: number,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'CONTINUE_ALARM', {
      id: id,
    })
  }

  async dismissAlarms<T>(
    ctx: Context<T>,
    ids: number[],
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'DISMISS_ALARMS', {
      ids: ids,
    })
  }

  async subscribe<T>(
    ctx: Context<T>,
  ): Promise<Error | [MessageHeader, MessageBody] | Abort<T | 'timeout'>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'SUBSCRIBE', {})
  }

  async kill<T>(ctx: Context<T>): Promise<undefined | Error | Abort<T | 'timeout'>> {
    const ctx2 = this.makeMessageContext(ctx)
    const ret = await this.sendMessage(ctx2, 'KILL', {})
    if (ret instanceof Error || ret instanceof Abort) {
      return ret
    }
    return undefined
  }

  addEventListener<T extends ConnectionEventType>(
    type: T,
    handler: (e: ConnectionEventMap[T]) => void,
  ): void {
    this.handlers[type].add(handler)
  }

  removeEventListener<T extends ConnectionEventType>(
    type: T,
    handler: (e: ConnectionEventMap[T]) => void,
  ): void {
    this.handlers[type].delete(handler)
  }

  get state(): 'open' | 'closed' | 'connecting' {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      return 'open'
    }
    return this.reconnecting ? 'connecting' : 'closed'
  }

  private emit<T extends ConnectionEventType>(
    type: T,
    data: IntersectingUnionIndex<ConnectionEventMap, T>,
  ): void {
    this.handlers[type].forEach((handler) => handler(data))
  }

  async reconnect(): Promise<undefined | Error> {
    if (this.reconnecting) {
      return this.reconnecting
    }
    if (
      this.socket &&
      this.socket.readyState !== WebSocket.CLOSED &&
      this.socket.readyState !== WebSocket.CLOSING
    ) {
      this.socket.close()
      this.socket = null
    }
    this.emit('reconnecting', undefined)
    const socket: MyWebSocket = new WebSocket('ws://localhost:8080/')
    let res: (_: boolean) => void
    const promIsSucc = new Promise<boolean>((res_) => (res = res_))
    let resReconnecting: (_: undefined) => void
    this.reconnecting = new Promise<undefined>((res_) => (resReconnecting = res_))
    socket.addEventListener('open', async (event) => res(true))
    socket.addEventListener('message', (event) => this.handleMessage(Context.background(), event))
    socket.addEventListener('close', (event) => {
      for (const res of this.promises.values()) {
        res(Error('Connection closed: ' + event.reason))
      }
      this.promises.clear()
      this.emit('closed', {
        weClosed: socket[weClosedSym] ?? false,
        code: event.code,
        reason: event.reason,
      })
    })
    socket.addEventListener('error', (error) => {
      res(false)
    })
    const isSucc = await promIsSucc
    this.reconnecting = null
    if (isSucc) {
      this.socket = socket
      this.emit('reconnected', undefined)
      resReconnecting!(undefined)
    }
    return
  }

  static async getConnectedInstance<T>(ctx: Context<T>): Promise<Connection | Error | Abort<T>> {
    const awaited = await Promise.race([Connection.connectedInstance, ctx.reason])
    if (awaited instanceof Connection) {
      return awaited
    }
    const c = Connection.getInstance()
    Connection.connectedInstance = new Promise<Error | Connection>(async (res) => {
      const r = await c.reconnect()
      if (r instanceof Error) {
        res(r)
      } else {
        res(c)
      }
    })
    return Promise.race([Connection.connectedInstance, ctx.reason])
  }

  static getInstance(): Connection {
    if (!Connection.instance) {
      Connection.instance = new Connection(null, new Map())
    }
    return Connection.instance
  }
}
