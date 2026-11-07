export type EventTarget<
  EventType extends string,
  ListenerArgs extends unknown[],
  Args extends unknown[],
  R,
> = {
  addEventListener(type: EventType, listener: (...args: ListenerArgs) => unknown, ...args: Args): R
  removeEventListener(
    type: EventType,
    callback: (...args: ListenerArgs) => unknown,
    ...args: Args
  ): unknown
}

export function addEventListenerWithLifetime<
  EventType extends string,
  ListenerArgs extends unknown[],
  Args extends unknown[],
  R,
>(
  onEndLifetime: (cb: () => void) => void,
  target: EventTarget<EventType, ListenerArgs, Args, R>,
  type: EventType,
  listener: (...args: ListenerArgs) => unknown,
  ...args: Args
): R {
  const ret = target.addEventListener(type, listener, ...args)
  onEndLifetime(() => target.removeEventListener(type, listener, ...args))
  return ret
}
