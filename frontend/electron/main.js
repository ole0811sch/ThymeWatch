import { app, BrowserWindow, session } from 'electron'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

function createWindow() {
  const win = new BrowserWindow({
    width: 800,
    height: 600,
    autoHideMenuBar: true,
    title: 'ThymeWatch',
  })
  if ((process.env.NODE_ENV ?? 'production') === 'production') {
    win.removeMenu()
  }

  win.loadFile(path.join(__dirname, '../dist-electron/index.html'))
}

app.whenReady().then(() => {
  session.defaultSession.webRequest.onBeforeSendHeaders((details, callback) => {
    const requestHeaders = {
      ...details.requestHeaders,
    }
    delete requestHeaders.Origin
    callback({ requestHeaders })
  })
  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow()
    }
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
