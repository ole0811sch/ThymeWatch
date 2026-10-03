export const tags = [
  'GET_CAPABILITIES',
  'CAPABILITIES',
  'ERROR',
  'SET_ALARM',
  'OK_RESPONSE',
] as const
export type Tag = (typeof tags)[number]
type MessageBody = {
  [x: string]: unknown
}
type MessageHead = {
  inResponseTo: number
  id: number
}
export type Message = MessageHead & MessageBody
export type PartialMessage = Partial<MessageHead> & MessageBody
export type ConnectionEventType = 'reconnected' | 'reconnecting' | 'closed' | 'setalarm'
export type ConnectionEventMap = {
  reconnecting: undefined
  reconnected: undefined
  closed: undefined
  setalarm: readonly [date: Date, name: string | undefined]
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
  }
  private constructor(
    private socket: WebSocket | null,
    private promises: Map<number, (response: [Tag, Message]) => void>,
  ) {}

  async sendMessage(tag: Tag, obj: PartialMessage): Promise<number | Error> {
    while (this.socket === null) {
      const res = await this.reconnect()
      if (res instanceof Error) {
        return res
      }
    }
    const id = this.i++
    this.socket.send(tag + ' ' + JSON.stringify({ id: id, inResponseTo: -1, ...obj }))
    return id
  }

  async asyncSendMessage(tag: Tag, obj: PartialMessage): Promise<[Tag, Message] | Error> {
    const id = await this.sendMessage(tag, obj)
    if (id instanceof Error) {
      return id
    }
    let res: (response: [Tag, Message]) => void
    const prom = new Promise<[Tag, Message]>((res_) => void (res = res_))
    this.promises.set(id, res!)
    return prom
  }

  close() {
    if (!this.socket) {
      return
    }
    this.socket.close()
  }

  private async handleMessage(event: MessageEvent<String>) {
    console.log('Message from server: ', event.data)
    const data = /**@type {string}*/ event.data
    const firstSpaceIdx = data.indexOf(' ')
    if (firstSpaceIdx === -1) {
      await this.sendMessage('ERROR', { message: 'Invalid message syntax' })
      return
    }
    const parts = [data.slice(0, firstSpaceIdx), data.slice(firstSpaceIdx + 1)] as const
    let obj: Record<string, unknown>
    try {
      obj = JSON.parse(parts[1])
    } catch (e) {
      console.error({
        invalidJson: parts[1],
      })
      await this.sendMessage('ERROR', { message: 'Invalid message json' })
      return
    }
    if (
      typeof obj.inResponseTo !== 'number' ||
      typeof obj.id !== 'number' ||
      !(tags as readonly string[]).includes(parts[0])
    ) {
      await this.sendMessage('ERROR', { message: 'Invalid message json' })
      return
    }
    const prom = this.promises.get(obj.inResponseTo)
    const tuple: [Tag, Message] = [
      parts[0] as Tag,
      { ...obj, inResponseTo: obj.inResponseTo, id: obj.id },
    ]
    if (prom !== undefined) {
      prom(tuple)
      return
    }
    switch (tuple[0]) {
      case 'SET_ALARM':
        if (obj.name !== undefined && typeof obj.name !== 'string') {
          await this.sendMessage('ERROR', { message: 'Invalid type of field `name`' })
          break
        }
        this.emit('setalarm', [new Date(obj.time as number), obj.name])
        await this.sendMessage('OK_RESPONSE', { inResponseTo: obj.id })
        break
      default:
        console.log(tuple)
    }
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
    const socket = new WebSocket('ws://localhost:8090/')
    let res: (_: boolean) => void
    const promIsSucc = new Promise<boolean>((res_) => (res = res_))
    let resReconnecting: (_: undefined) => void
    this.reconnecting = new Promise<undefined>((res_) => (resReconnecting = res_))
    socket.addEventListener('open', async (event) => res(true))
    socket.addEventListener('message', (event) => this.handleMessage(event))
    socket.addEventListener('close', (event) => {
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
