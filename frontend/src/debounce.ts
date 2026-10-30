import { type Ref, ref, watch } from 'vue'

/**
 * @param opts
 *  doUpdateImmediately: If it returns true for some value, that value
 *    will be set immediately, even if there was an update recently. Is called
 *    exactly once per update to origRef that `watch()` reported.
 *  alwaysDelay: Delay all updates regardless of when the last update was.
 *    doUpdateImmediately takes precendence however, so if it is specified and
 *    returns true for a value, the update will still be propagated immediately
 */
export function debounceRef<T>(
  origRef: Ref<T>,
  delayMs: number,
  opts?: {
    doUpdateImmediately?: (newValue: T, oldValue: T) => boolean
    alwaysDelay?: boolean
  },
) {
  opts ??= {}
  let lastChange: Date = new Date(0)
  let timeoutId: number | null = null
  function updateIsConnecting() {
    if (timeoutId !== null) {
      clearTimeout(timeoutId)
      timeoutId = null
    }
    lastChange = new Date()
    debouncedRef.value = origRef.value
  }
  const debouncedRef = ref(origRef.value)
  watch(origRef, (newValue, oldValue) => {
    if (
      (opts.doUpdateImmediately && opts.doUpdateImmediately(newValue, oldValue)) ||
      (!opts.alwaysDelay && lastChange.getTime() + delayMs <= Date.now())
    ) {
      updateIsConnecting()
    } else if (timeoutId === null) {
      timeoutId = setTimeout(() => {
        updateIsConnecting()
      }, delayMs)
    }
  })
  return debouncedRef
}

export function temporarilyFixRef<T>(origRef: Ref<T>, value: T, forMs: number) {
  const fixedRef = ref(value)
  setTimeout(() => {
    watch(origRef, (value) => (fixedRef.value = value), { immediate: true })
  }, forMs)
  return fixedRef
}
