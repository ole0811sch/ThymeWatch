import type { ElectronApi } from '../../interface/types'

declare global {
  interface Window {
    electronApi?: ElectronApi
  }
}

export type UnionOfTuplesToIntersection<U extends [unknown]> = (
  U extends [infer V] ? (arg: V) => void : never
) extends (arg: infer T) => void
  ? T
  : never
export type IntersectingUnionIndex<T, U extends keyof T> = UnionOfTuplesToIntersection<
  U extends unknown ? [T[U]] : never
> &
  T[U]
