export type SetAlarmArg = {
  expiryTime: Date
  name?: string
}

export type ElectronApi = {
  onSetAlarm: (callback: (config: SetAlarmArg) => Promise<void> | undefined) => void
}
