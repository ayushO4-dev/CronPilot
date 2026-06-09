// Types mirroring the Go JSON API.

export interface User {
  id: string
  username: string
  mustChangePassword: boolean
}

export interface Settings {
  theme: 'dark' | 'light'
  sessionIdleSeconds: number
  sessionMaxSeconds: number
  dev: boolean
  version: string
}

export interface HostInfo {
  hostname: string
  os: string
  platform: string
  platformVersion: string
  kernelVersion: string
  kernelArch: string
  uptime: number
  bootTime: number
  procs: number
}

export interface CPUInfo {
  cores: number
  physicalCores: number
  modelName: string
  mhz: number
  percent: number
  perCore: number[]
}

export interface MemInfo {
  total: number
  used: number
  available: number
  usedPercent: number
}

export interface SwapInfo {
  total: number
  used: number
  usedPercent: number
}

export interface LoadInfo {
  load1: number
  load5: number
  load15: number
}

export interface DiskInfo {
  device: string
  mountpoint: string
  fstype: string
  total: number
  used: number
  free: number
  usedPercent: number
}

export interface NetInfo {
  name: string
  bytesSent: number
  bytesRecv: number
}

export interface Summary {
  time: number
  host: HostInfo
  cpu: CPUInfo
  memory: MemInfo
  swap: SwapInfo
  load?: LoadInfo
  disks: DiskInfo[] | null
  networks: NetInfo[] | null
}

export interface Sample {
  time: number
  cpuPercent: number
  perCore: number[] | null
  memUsed: number
  memTotal: number
  memUsedPercent: number
  swapUsed: number
  swapTotal: number
  swapUsedPercent: number
  load1: number
  netRxBytesPerSec: number
  netTxBytesPerSec: number
}

export interface ServiceUnit {
  name: string
  description: string
  loadState: string
  activeState: string
  subState: string
  enabled: string
}

export interface ServiceDetail extends ServiceUnit {
  fragmentPath: string
  mainPID: number
  memoryCurrent: number
  since: string
}

export interface ProcessInfo {
  pid: number
  ppid: number
  name: string
  user: string
  cpuPercent: number
  memoryPercent: number
  rss: number
  status: string
  cmdline: string
  createTime: number
}

export interface ProcessDetail extends ProcessInfo {
  exe: string
  cwd: string
  numThreads: number
  nice: number
}

export type TriggerType = 'interval' | 'cron' | 'manual'

export interface Trigger {
  type: TriggerType
  intervalSeconds?: number
  cron?: string
}

export type MatchMode = 'all' | 'any'

export interface Contact {
  id?: string
  kind: string
  negate?: boolean
  params: Record<string, unknown>
}

export interface Action {
  id?: string
  kind: string
  params: Record<string, unknown>
}

export interface Rung {
  id?: string
  label?: string
  match: MatchMode
  contacts: Contact[]
  actions: Action[]
}

export interface Task {
  id: string
  name: string
  description?: string
  enabled: boolean
  trigger: Trigger
  runAs?: string
  rungs: Rung[]
  createdAt?: string
  updatedAt?: string
  lastRun?: string | null
  lastStatus?: string
}

export interface TaskRun {
  id: number
  taskId: string
  time: string
  trigger: string
  ok: boolean
  summary: string
  detail?: string
  durationMs: number
}
