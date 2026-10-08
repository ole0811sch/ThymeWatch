import { BrowserWindow, ipcMain } from 'electron'
import express from 'express'
import assert from 'node:assert/strict'
import { exit } from 'node:process'
import { setTimeout } from 'node:timers/promises'

/**
 * @import { SetAlarmArg } from '../interface/types'
 * @import { WebContents } from 'electron'
 */

let counter = 0
/** @type {Map<number, (v: any) => any>} */
let promiseResolvers = new Map()
/** @type {Map<WebContents, Set<number>>} */
let windowToPromiseResolverIds = new Map()
/**@type {Set<WebContents>} */
const webContentss = new Set()

/**
 * @param {WebContents} webContents
 * @param {SetAlarmArg} config
 * @returns {Promise<void>}
 */
function setAlarm(webContents, config) {
  /** @type {(v: void) => void=} */
  let res
  const prom = new Promise((res_) => (res = res_))
  assert(res)
  const id = counter++
  if (id) {
    // exit(1)
  }
  webContents.send('set-alarm', config, id)
  windowToPromiseResolverIds.getOrInsertComputed(webContents, () => new Set()).add(id)
  promiseResolvers.set(id, res)

  return prom
}

ipcMain.addListener('set-alarm-ack', (_event, id) => {
  const res = promiseResolvers.get(id)
  if (!res) {
    console.warn('set-alarm-ack: received ACK with id for which no resolver is registered')
    return
  }
  res(undefined)
})

ipcMain.addListener('set-alarm-hello', (event) => {
  registerWebContents(event.sender)
})

const app = express()
app.post('/alarm', async (req, res) => {
  if (typeof req.query.expirytime !== 'string') {
    return void res.sendStatus(400)
  }
  const expiryTime = new Date(Number(req.query.expirytime))
  if (isNaN(expiryTime.getTime())) {
    return void res.sendStatus(400)
  }
  /**@type {string=} */
  let name
  if (req.query.name !== undefined) {
    if (typeof req.query.name !== 'string') {
      return void res.sendStatus(400)
    }
    name = req.query.name
  }
  if (webContentss.size === 0) {
    return void res.sendStatus(503)
  }
  /** @type {Promise<void>[]} */
  const proms = []
  for (const wc of webContentss) {
    proms.push(
      setAlarm(wc, {
        expiryTime: expiryTime,
        name: name,
      }),
    )
  }
  proms.push()
  const awaitedProms = await Promise.any([
    Promise.all(proms),
    setTimeout(1000 * 5).then(() => /**@type {const}*/ ('timeout')),
  ])
  if (awaitedProms === 'timeout') {
    await Promise.any(proms)
  }
  res.sendStatus(200)
})

/**
 * @returns {Promise<void>}
 */
export function startServer() {
  return new Promise((res, rej) =>
    app.listen(8091, '127.0.0.1', (err) => {
      if (err) {
        rej(err)
      }
      console.log('Listening for local connections on port 8091')
      res()
    }),
  )
}

/**
 * @param {WebContents} webContents
 */
export function registerWebContents(webContents) {
  webContentss.add(webContents)
  webContents.addListener('destroyed', () => {
    webContentss.delete(webContents)
    for (const id of windowToPromiseResolverIds.get(webContents) ?? []) {
      promiseResolvers.delete(id)
    }
    windowToPromiseResolverIds.delete(webContents)
  })
}
