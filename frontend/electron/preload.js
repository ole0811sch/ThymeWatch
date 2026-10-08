/* eslint-disable @typescript-eslint/no-require-imports */
const { contextBridge, ipcRenderer } = require('electron')

/** @import { SetAlarmArg } from '../interface/types' */

/** @typedef {typeof electronApi} ElectronApi */

const electronApi = {
  /**
   * @param {(config: SetAlarmArg) => Promise<void> | undefined} callback
   * @returns {void}
   */
  onSetAlarm: (callback) => {
    ipcRenderer.send('set-alarm-hello')
    ipcRenderer.on('set-alarm', async (_event, value, id) => {
      await callback(value)
      ipcRenderer.send('set-alarm-ack', id)
    })
  },
}

contextBridge.exposeInMainWorld('electronApi', electronApi)
