import { Abort, Context, type TimeoutContext } from './context'

export const tags = [
  'CREATE_ALARM',
  'CANCEL_ALARM',
  'DISMISS_ALARMS',
  'SUBSCRIBE',
  'NOTIFY_ALARM',
  'ALARM_CANCELED',
  'ALARM_EXPIRED',
  'ALARMS_DISMISSED',
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
/**
 * ketchup is fired when the server resends all alarms after reconnect. It's
 * fired before any of the alarms' events are fired
 */
export type ConnectionEventType =
  | 'reconnected'
  | 'reconnecting'
  | 'closed'
  | 'setalarm'
  | 'cancelalarm'
  | 'expirealarm'
  | 'alarmsdismissed'
  | 'ketchup'
export type ConnectionEventMap = {
  reconnecting: undefined
  reconnected: undefined
  closed: undefined
  setalarm: {
    expiryTime: Date
    name: string | undefined
    id: number
    startedAt: Date
    ketchup: boolean
  }
  cancelalarm: number
  expirealarm: number
  alarmsdismissed: number[]
  ketchup: undefined
}

export class Connection {
  private i = 0
  private static connectedInstance: Promise<Connection | Error> | null = null
  private static instance: Connection | null = null
  private messageTimeoutMs: number = 1000
  private reconnecting: Promise<undefined> | null = null
  private handlers: {
    [T in ConnectionEventType]: Set<(data: ConnectionEventMap[T]) => void>
  } = {
    closed: new Set(),
    reconnected: new Set(),
    reconnecting: new Set(),
    setalarm: new Set(),
    cancelalarm: new Set(),
    expirealarm: new Set(),
    alarmsdismissed: new Set(),
    ketchup: new Set(),
  }
  private constructor(
    private socket: WebSocket | null,
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
    return Promise.race([prom, ctx.reason])
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

    switch (header.tag) {
      case 'NOTIFY_ALARM': {
        const expiryDates: Date[] = []
        const startedAts: Date[] = []
        if (
          typeof body.ketchup !== 'boolean' ||
          !Array.isArray(body.alarms) ||
          body.alarms.some((alarm) => {
            if (
              typeof alarm.id !== 'number' ||
              typeof alarm.spec !== 'object' ||
              (alarm.spec.name !== undefined && typeof alarm.spec.name !== 'string') ||
              typeof alarm.spec.expiryTime !== 'string' ||
              typeof alarm.spec.startedAt !== 'string'
            ) {
              return true
            }
            const expiryDate = new Date(alarm.spec.expiryTime)
            const startedAt = new Date(alarm.spec.startedAt)
            if (isNaN(expiryDate.getTime()) || isNaN(startedAt.getTime())) {
              return true
            }
            expiryDates.push(expiryDate)
            startedAts.push(startedAt)
          })
        ) {
          console.error('Invalid payload for NOTIFY_ALARM')
          break
        }
        if (body.ketchup) {
          this.emit('ketchup', undefined)
        }
        for (let i = 0; i < body.alarms.length; ++i) {
          this.emit('setalarm', {
            expiryTime: expiryDates[i]!,
            name: body.alarms[i].spec.name,
            id: body.alarms[i].id,
            startedAt: startedAts[i]!,
            ketchup: body.ketchup,
          })
        }
        const ctx2 = this.makeMessageContext(ctx)
        await this.sendMessage(ctx2, header.id, { status: 200 })
        ctx2.cancel()
        break
      }
      case 'ALARM_CANCELED':
      case 'ALARM_EXPIRED': {
        if (typeof body.id !== 'number') {
          console.error('Invalid type of field `id`')
          break
        }
        this.emit(header.tag === 'ALARM_CANCELED' ? 'cancelalarm' : 'expirealarm', body.id)
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

  async cancelAlarm<T>(
    ctx: Context<T>,
    id: number,
  ): Promise<[MessageHeader, MessageBody] | Error | Abort<'timeout' | T>> {
    return await this.asyncSendMessageDefaultTimeout(ctx, 'CANCEL_ALARM', {
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

  private emit<T extends ConnectionEventType>(type: T, data: ConnectionEventMap[T]): void {
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
    const socket = new WebSocket('ws://localhost:8080/')
    let res: (_: boolean) => void
    const promIsSucc = new Promise<boolean>((res_) => (res = res_))
    let resReconnecting: (_: undefined) => void
    this.reconnecting = new Promise<undefined>((res_) => (resReconnecting = res_))
    socket.addEventListener('open', async (event) => res(true))
    socket.addEventListener('message', (event) => this.handleMessage(Context.background(), event))
    socket.addEventListener('close', (event) => {
      if (event.code !== 1000) {
        console.log('Connection closed: ' + event.reason)
      }
      for (const res of this.promises.values()) {
        res(Error('Connection closed: ' + event.reason))
      }
      this.promises.clear()
      this.emit('closed', undefined)
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
