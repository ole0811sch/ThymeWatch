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
  private static instance: Promise<Connection> | null = null
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

  async sendMessage(tagOrReplyId: Tag | number, obj: MessageBody): Promise<number | Error> {
    while (this.socket === null) {
      const res = await this.reconnect()
      if (res instanceof Error) {
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

  async asyncSendMessage(
    tagOrReplyId: Tag | number,
    obj: MessageBody,
  ): Promise<[MessageHeader, MessageBody] | Error> {
    const id = await this.sendMessage(tagOrReplyId, obj)
    if (id instanceof Error) {
      return id
    }
    let res: (response: [MessageHeader, MessageBody] | Error) => void
    const prom = new Promise<[MessageHeader, MessageBody] | Error>((res_) => void (res = res_))
    this.promises.set(id, res!)
    return prom
  }

  close() {
    if (!this.socket) {
      return
    }
    this.socket.close()
  }

  private async handleMessage(event: MessageEvent<string>) {
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
      case 'NOTIFY_ALARM':
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
        await this.sendMessage(header.id, { status: 200 })
        break
      case 'ALARM_CANCELED':
      case 'ALARM_EXPIRED':
        if (typeof body.id !== 'number') {
          console.error('Invalid type of field `id`')
          break
        }
        this.emit(header.tag === 'ALARM_CANCELED' ? 'cancelalarm' : 'expirealarm', body.id)
        await this.sendMessage(header.id, { status: 200 })
        break
      case 'ALARMS_DISMISSED':
        if (!Array.isArray(body.ids) || body.ids.some((id) => typeof id !== 'number')) {
          console.error('Invalid type of field `id`')
          break
        }
        this.emit('alarmsdismissed', body.ids)
        await this.sendMessage(header.id, { status: 200 })
        break
      default:
        console.log(tuple)
    }
  }

  async createAlarm(expiryTime: Date, startedAt: Date, name?: string) {
    return await this.asyncSendMessage('CREATE_ALARM', {
      name: name,
      expiryTime: expiryTime,
      startedAt: startedAt,
    })
  }

  async cancelAlarm(id: number) {
    return await this.asyncSendMessage('CANCEL_ALARM', {
      id: id,
    })
  }

  async dismissAlarms(ids: number[]) {
    return await this.asyncSendMessage('DISMISS_ALARMS', {
      ids: ids,
    })
  }

  async subscribe() {
    return await this.asyncSendMessage('SUBSCRIBE', {})
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
    socket.addEventListener('message', (event) => this.handleMessage(event))
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

  static getInstance(): Promise<Connection> {
    if (Connection.instance) {
      return Connection.instance
    }
    const c = new Connection(null, new Map())
    Connection.instance = new Promise(async (res) => {
      await c.reconnect()
      res(c)
    })
    return Connection.instance
  }
}
