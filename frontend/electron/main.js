import { app, BrowserWindow } from 'electron'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import { registerWebContents, startServer } from './server.js'

if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  const __dirname = path.dirname(fileURLToPath(import.meta.url))

  /** @type {BrowserWindow=} */
  let window

  function createWindow() {
    window = new BrowserWindow({
      width: 800,
      height: 600,
      autoHideMenuBar: true,
      title: 'ThymeWatch',
      webPreferences: {
        preload: path.join(__dirname, 'preload.js'),
      },
    })
    if ((process.env.NODE_ENV ?? 'production') === 'production') {
      window.removeMenu()
    }

    window.loadFile(path.join(__dirname, '../dist-electron/index.html'))
  }

  app.whenReady().then(async () => {
    createWindow()
    await startServer()

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        createWindow()
      }
    })
  })

  app.on('window-all-closed', () => {
    window = undefined
  })

  app.on('second-instance', () => {
    if (window) {
      if (window.isMinimized()) {
        window.restore()
      }
      window.focus()
    } else {
      createWindow()
    }
  })
}
