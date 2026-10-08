import type { ElectronApi } from '../interface/types'

declare global {
  interface Window {
    electronApi?: ElectronApi
  }
}
