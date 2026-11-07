export class TimeoutError extends Error {
  constructor() {
    super('The allocated time was exceeded')
  }
}

export class Abort<T> {
  readonly value: T
  constructor(value: T) {
    this.value = value
  }
}

export function withTimeout<T>(p: Promise<T>, millis: number): Promise<T | TimeoutError> {
  let res
  const timeoutProm = new Promise<TimeoutError>((res_) => (res = res_))
  const clearId = setTimeout(() => res!(new TimeoutError()), millis)
  return Promise.race([p, timeoutProm]).finally(() => clearTimeout(clearId))
}

export type TimeoutContext = Context<'timeout'>
export type WOU<T> = Exclude<T, undefined>

export class Context<T = true> {
  private abortController: Omit<AbortController, 'abort'> & {
    abort: (this: AbortController, reason: WOU<T>) => void
  }
  private clearId?: number
  private isCanceled = false
  /**
   * Resolves with the abort reason
   */
  readonly reason: Promise<Abort<WOU<T>>>
  private constructor() {
    this.abortController = new AbortController()
    this.reason = new Promise((res) =>
      this.abortController.signal.addEventListener('abort', function (ev) {
        res(new Abort(this.reason as WOU<T>))
      }),
    )
  }
  private static inherited<T, U>(ctx?: Context<T>): Context<T | U> {
    const ctx2 = new Context<T | U>()
    ctx?.reason.then((r) => ctx2.abort(r.value))
    return ctx2
  }
  static background<T = never>(): Context<T> {
    return new Context()
  }
  static withCancel<T = true>(): Context<T> {
    return new Context()
  }
  /**
   * @returns Context that will abort with reason TimeoutError after millis ms.
   */
  static withTimeout<T = never, U = never>(
    millis: number,
    ctx?: Context<T>,
  ): Context<T | U | 'timeout'> {
    const ctx2 = Context.inherited<T, U | 'timeout'>(ctx)
    ctx2.reason.then(() => {
      if (ctx2.clearId !== undefined) {
        clearTimeout(ctx2.clearId)
      }
    })
    ctx2.clearId = setTimeout(() => {
      ctx2.clearId = undefined
      ctx2.abortController.abort('timeout')
    }, millis)
    return ctx2
  }
  on(expectedReason: WOU<T>, callback: () => void) {
    this.reason.then((reason) => {
      if (this.isCanceled) {
        return
      }
      if (reason === expectedReason) {
        callback()
      }
    })
  }

  onTimeout(
    this: this extends Context<infer T> ? ('timeout' extends T ? this : never) : never,
    callback: () => void,
  ) {
    this.on('timeout' as WOU<T>, callback)
  }

  abort(this: this extends Context<infer U> ? (true extends U ? this : never) : never): void
  abort(reason: WOU<T>): void
  abort(reason?: WOU<T>) {
    if (this.isCanceled) {
      return
    }
    this.abortController.abort(reason === undefined ? (true as WOU<T>) : reason)
  }
  /**
   * Prevents from being aborted
   */
  cancel(): void {
    this.isCanceled = true
    if (this.clearId !== undefined) {
      clearTimeout(this.clearId)
    }
  }
  get signal(): AbortSignal {
    return this.abortController.signal
  }
  get maybeReason(): WOU<T> | null {
    return this.abortController.signal.reason
  }
}
