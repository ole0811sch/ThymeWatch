import { app, BrowserWindow, session } from 'electron'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

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
  session.defaultSession.webRequest.onBeforeSendHeaders((details, callback) => {
    const requestHeaders = {
      ...details.requestHeaders,
      Origin: 'org.thymewatch://',
    }
    callback({ requestHeaders })
  })
  createWindow()

  app.on('activate', () => {
    if (!window) {
      createWindow()
    }
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
  window = undefined
})
