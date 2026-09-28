export type User = {
  id: string
  name: string
  email: string
  role: string
  status: string
  last_seen_at: string
  created_at: string
}
export type Agent = {
  id: string
  owner_id: string
  name: string
  subtitle: string
  persona: string
  color: string
  location: string
  activity: string
  universe_id: string
  provider_id: string | null
  version: number
}
export type World = {
  id: string
  name: string
  kind: string
  description: string
  population: number
}
export type Message = {
  id: string
  role: string
  content: string
  state: string
  created_at: string
}
export type Memory = {
  id: string
  content: string
  category: string
  source: string
  created_at: string
  universe_id: string | null
}
export type Provider = {
  id: string
  name: string
  protocol: string
  base_url: string
  model: string
  key_set: boolean
}
export type WorldEvent = {
  id: string
  title: string
  content: string
  kind: string
  created_at: string
  actor_ids: string[]
}
export type Post = {
  id: string
  author_name: string
  author_color: string
  content: string
  universe_id: string
  agent_id: string | null
  event_id: string | null
  created_at: string
}
export type Task = {
  id: string
  goal: string
  agent_id: string
  state: string
  result: string
  created_at: string
  updated_at: string
  steps: Step[]
  artifacts?: Artifact[]
}
export type Step = {
  index: number
  tool: string
  path?: string
  state: string
  at: string
  result: Record<string, unknown>
}
export type Artifact = {
  id: string
  name: string
  media_type: string
  version: number
  content?: string
  created_at: string
}
export type AdminUser = User & {
  presence: string
  agent_count: number
  active_tasks: number
  failed_tasks: number
  status_updated_at: string
  agents?: Agent[]
  tasks?: Task[]
  universes?: World[]
}
export type Overview = {
  users: number
  online: number
  agents: number
  active_tasks: number
  failed_tasks: number
  universes: number
  at: string
}
export type Audit = {
  id: string
  action: string
  target_id: string
  created_at: string
  detail: Record<string, string>
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch('/api' + path, {
    ...options,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-Universe-Client': 'web', ...options.headers },
  })
  const data = await response.json().catch(() => ({ detail: '服务返回了无法读取的内容' }))
  if (!response.ok)
    throw new Error(
      typeof data.detail === 'string' ? data.detail : data.detail?.[0]?.msg || '操作失败，请重试',
    )
  return data
}
export const post = <T>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: JSON.stringify(body ?? {}) })
export const formatTime = (s: string) =>
  new Date(s).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
export const formatDate = (s: string) =>
  new Date(s).toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
export const taskLabels: Record<string, string> = {
  queued: '等待开始',
  running: '进行中',
  succeeded: '已完成',
  failed: '执行失败',
  cancelled: '已取消',
  waiting_configuration: '等待配置模型',
  blocked: '需要更多能力',
}
