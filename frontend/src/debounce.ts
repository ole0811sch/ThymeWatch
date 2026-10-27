import { type Ref, ref, watch } from 'vue'

/**
 * @param doUpdateImmediately If it returns true for some value, that value
 * will be set immediately, even if there was an update recently. Is called
 * exactly once per update to origRef that `watch()` reported.
 */
export function debounceRef<T>(
  origRef: Ref<T>,
  delayMs: number,
  doUpdateImmediately?: (newValue: T, oldValue: T) => boolean,
) {
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
      (doUpdateImmediately && doUpdateImmediately(newValue, oldValue)) ||
      lastChange.getTime() + delayMs <= Date.now()
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
