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

export class Connection {
  private i = 0
  private constructor(
    private socket: WebSocket,
    private promises: Map<number, (response: [Tag, Message]) => void>,
    private onSetAlarm: (date: Date) => void,
  ) {}

  sendMessage(tag: Tag, obj: PartialMessage) {
    const id = this.i++
    this.socket.send(tag + ' ' + JSON.stringify({ id: id, inResponseTo: -1, ...obj }))
    return id
  }

  asyncSendMessage(tag: Tag, obj: PartialMessage): Promise<[Tag, Message]> {
    const id = this.sendMessage(tag, obj)
    let res: (response: [Tag, Message]) => void
    const prom = new Promise<[Tag, Message]>((res_) => void (res = res_))
    this.promises.set(id, res!)
    return prom
  }

  close() {
    this.socket.close()
  }

  private handleMessage(event: MessageEvent<String>) {
    console.log('Message from server: ', event.data)
    const data = /**@type {string}*/ event.data
    const firstSpaceIdx = data.indexOf(' ')
    if (firstSpaceIdx === -1) {
      this.sendMessage('ERROR', { message: 'Invalid message syntax' })
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
      this.sendMessage('ERROR', { message: 'Invalid message json' })
      return
    }
    if (
      typeof obj.inResponseTo !== 'number' ||
      typeof obj.id !== 'number' ||
      !(tags as readonly string[]).includes(parts[0])
    ) {
      this.sendMessage('ERROR', { message: 'Invalid message json' })
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
        this.onSetAlarm(new Date(obj.time as number))
        this.sendMessage('OK_RESPONSE', { inResponseTo: obj.id })
        break
      default:
        console.log(tuple)
    }
  }

  static async new(onSetAlarm: (date: Date) => void): Promise<Connection> {
    // Creates a new WebSocket connection to the specified URL.
    const socket = new WebSocket('ws://localhost:8090/')
    const promises = new Map<number, (response: [Tag, Message]) => void>()

    const c = new Connection(socket, promises, onSetAlarm)
    let res: () => void
    const prom = new Promise<void>((res_) => (res = res_))
    // Executes when the connection is successfully established.
    socket.addEventListener('open', async (event) => res())
    socket.addEventListener('open', async (event) => {})
    // Listen for messages and executes when a message is received from the server.
    socket.addEventListener('message', (event) => c.handleMessage(event))
    // Executes when the connection is closed, providing the close code and reason.
    socket.addEventListener('close', (event) => {
      console.log('WebSocket connection closed:', event.code, event.reason)
    })
    // Executes if an error occurs during the WebSocket communication.
    socket.addEventListener('error', (error) => {
      console.error('WebSocket error:', error)
    })
    return c
  }
}
