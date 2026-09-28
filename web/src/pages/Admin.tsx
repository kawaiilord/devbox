import { useState, useCallback, useEffect } from 'react'
import type { FormEvent } from 'react'
import {
  Orbit,
  ShieldCheck,
  ChevronDown,
  ChevronRight,
  Users,
  ArrowLeft,
  LogOut,
  Clock3,
  MessageCircle,
  Sparkles,
  WandSparkles,
  Search,
  FileText,
} from 'lucide-react'
import { Avatar, Modal } from '../components'
import { api, formatDate, formatTime, taskLabels } from '../api'
import type { AdminUser, Audit, Overview, User } from '../api'
export default function Admin({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [tab, setTab] = useState<'users' | 'audit'>('users'),
    [overview, setOverview] = useState<Overview | null>(null),
    [users, setUsers] = useState<AdminUser[]>([]),
    [audits, setAudits] = useState<Audit[]>([]),
    [search, setSearch] = useState(''),
    [detail, setDetail] = useState<AdminUser | null>(null),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const refresh = useCallback(async () => {
    const [o, u, a] = await Promise.all([
      api<Overview>('/admin/overview'),
      api<AdminUser[]>('/admin/users?q=' + encodeURIComponent(search)),
      api<Audit[]>('/admin/audits'),
    ])
    setOverview(o)
    setUsers(u)
    setAudits(a)
  }, [search])
  useEffect(() => {
    if (user.role !== 'admin') return
    refresh().catch((e) => setError(e.message))
    const i = setInterval(() => {
      refresh().catch((e) => setError(e.message))
    }, 15000)
    return () => clearInterval(i)
  }, [refresh, user.role])
  const closeDetail = useCallback(() => setDetail(null), [])
  if (user.role !== 'admin')
    return (
      <div className="empty-state">
        <ShieldCheck size={40} />
        <h2>此页面需要管理员权限</h2>
        <a className="button primary" href="/">
          返回邻宇
        </a>
      </div>
    )
  async function toggleStatus(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!detail) return
    setBusy(true)
    setError('')
    const reason = new FormData(e.currentTarget).get('reason')
    try {
      await api('/admin/users/' + detail.id + '/status', {
        method: 'PATCH',
        body: JSON.stringify({
          status: detail.status === 'active' ? 'suspended' : 'active',
          reason,
        }),
      })
      setDetail(await api<AdminUser>('/admin/users/' + detail.id))
      await refresh()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="admin-shell">
      <aside className="admin-sidebar">
        <a className="wordmark" href="/">
          <Orbit size={27} />
          <b>邻宇</b>
          <span>CONSOLE</span>
        </a>
        <div className="admin-space">
          <span className="live-dot" />
          平台工作空间
          <ChevronDown size={15} />
        </div>
        <span className="admin-nav-label">平台管理</span>
        <button className={tab === 'users' ? 'active' : ''} onClick={() => setTab('users')}>
          <Users size={19} />
          用户与运行状态
        </button>
        <button className={tab === 'audit' ? 'active' : ''} onClick={() => setTab('audit')}>
          <ShieldCheck size={19} />
          操作审计<span>{audits.length}</span>
        </button>
        <div className="admin-nav-bottom">
          <a href="/">
            <ArrowLeft size={17} />
            返回用户端
          </a>
          <button onClick={onLogout}>
            <LogOut size={17} />
            退出登录
          </button>
          <div>
            <span className="self-avatar">{user.name[0]}</span>
            <span>
              {user.name}
              <small>平台管理员</small>
            </span>
          </div>
        </div>
      </aside>
      <div className="admin-body">
        <header className="admin-topbar">
          <span>
            平台管理
            <ChevronRight size={14} />
            <b>{tab === 'users' ? '用户与运行状态' : '操作审计'}</b>
          </span>
          <span>
            <span className="live-dot" />
            数据已连接
            <ShieldCheck size={18} />
          </span>
        </header>
        <main className="admin-main">
          <div className="page-heading">
            <div>
              <span className="eyebrow">PLATFORM OBSERVABILITY</span>
              <h1>{tab === 'users' ? '每一位用户，都清晰可见。' : '每一次操作，都有迹可循。'}</h1>
              <p>账户、在线情况与执行活动独立呈现，状态来自实际运行数据。</p>
            </div>
            <button
              className="button subtle"
              onClick={() => refresh().catch((e) => setError(e.message))}
            >
              刷新数据
              <Clock3 size={15} />
            </button>
          </div>
          {error && (
            <div className="form-error" role="alert">
              {error}
            </div>
          )}
          <div className="metric-grid">
            {[
              { label: '平台用户', value: overview?.users, icon: Users, hint: '已注册账户' },
              {
                label: '当前在线',
                value: overview?.online,
                icon: MessageCircle,
                hint: '最近 90 秒有活动',
              },
              { label: 'AI 好友', value: overview?.agents, icon: Sparkles, hint: '用户拥有的角色' },
              {
                label: '执行中任务',
                value: overview?.active_tasks,
                icon: WandSparkles,
                hint: '包含队列中的任务',
              },
            ].map(({ label, value, icon: Icon, hint }) => (
              <div className="metric" key={label}>
                <div>
                  <span>{label}</span>
                  <Icon size={18} />
                </div>
                <b>{value ?? '—'}</b>
                <small>{hint}</small>
              </div>
            ))}
          </div>
          {tab === 'users' ? (
            <section className="admin-table-card">
              <div className="admin-table-top">
                <div>
                  <h2>用户状态</h2>
                  <span>{users.length} 条记录</span>
                </div>
                <div className="search-field">
                  <Search size={16} />
                  <input
                    aria-label="搜索用户"
                    placeholder="搜索姓名、邮箱或用户 ID"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </div>
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>用户</th>
                      <th>账户状态</th>
                      <th>在线情况</th>
                      <th>AI 好友</th>
                      <th>任务</th>
                      <th>最近活动</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((u) => (
                      <tr key={u.id}>
                        <td>
                          <div className="table-user">
                            <span className="table-avatar">{u.name[0]}</span>
                            <span>
                              <b>
                                {u.name}
                                {u.role === 'admin' && <small className="role-badge">管理员</small>}
                              </b>
                              <small>{u.email}</small>
                            </span>
                          </div>
                        </td>
                        <td>
                          <span
                            className={
                              'status ' + (u.status === 'active' ? 'succeeded' : 'cancelled')
                            }
                          >
                            {u.status === 'active' ? '正常' : '已停用'}
                          </span>
                        </td>
                        <td>
                          <span className={'presence ' + u.presence}>
                            <i />
                            {
                              (
                                { online: '在线', idle: '空闲', offline: '离线' } as Record<
                                  string,
                                  string
                                >
                              )[u.presence]
                            }
                          </span>
                        </td>
                        <td>{u.agent_count}</td>
                        <td>
                          {u.active_tasks ? (
                            <span className="status running">{u.active_tasks} 进行中</span>
                          ) : (
                            <span className="table-muted">暂无进行中任务</span>
                          )}
                        </td>
                        <td className="table-time">{formatDate(u.last_seen_at)}</td>
                        <td>
                          <button
                            className="text-button"
                            onClick={() =>
                              api<AdminUser>('/admin/users/' + u.id)
                                .then(setDetail)
                                .catch((e) => setError(e.message))
                            }
                          >
                            查看
                            <ChevronRight size={14} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {users.length === 0 && <div className="empty-inline">没有符合搜索条件的用户。</div>}
              <div className="table-footer">
                <span>
                  <Clock3 size={13} />
                  更新时间：{overview ? formatTime(overview.at) : '—'}
                </span>
                <span>用户离线后，AI 仍可继续虚拟生活</span>
              </div>
            </section>
          ) : (
            <section className="admin-table-card">
              <div className="admin-table-top">
                <h2>操作记录</h2>
                <span className="muted-small">迁移与账户管理操作</span>
              </div>
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>操作</th>
                      <th>目标 ID</th>
                      <th>记录详情</th>
                      <th>发生时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    {audits.map((a) => (
                      <tr key={a.id}>
                        <td>
                          <span className="tag">{a.action}</span>
                        </td>
                        <td className="mono">{a.target_id.slice(0, 13)}…</td>
                        <td>{a.detail.reason || '宇宙归属变更'}</td>
                        <td>{formatDate(a.created_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!audits.length && <div className="empty-inline">还没有管理操作记录。</div>}
              </div>
            </section>
          )}
          <div className="admin-bottom-note">
            <ShieldCheck size={16} />
            状态视图按权限提供运行信息，管理操作会留下审计记录。<span>邻宇管理平台 · 0.1</span>
          </div>
        </main>
      </div>
      {detail && (
        <Modal title="用户详情" onClose={closeDetail} wide>
          <div className="admin-detail">
            <div className="table-user">
              <span className="table-avatar large">{detail.name[0]}</span>
              <span>
                <h2>{detail.name}</h2>
                <p>{detail.email}</p>
                <small className="mono">{detail.id}</small>
              </span>
            </div>
            <div className="detail-summary">
              <span>账户：{detail.status === 'active' ? '正常' : '已停用'}</span>
              <span>角色：{detail.agent_count}</span>
              <span>运行任务：{detail.active_tasks}</span>
              <span>失败任务：{detail.failed_tasks}</span>
            </div>
            <h3>AI 好友与归属</h3>
            {detail.agents?.map((a) => (
              <div className="resident" key={a.id}>
                <Avatar name={a.name} color={a.color} size="small" />
                <div>
                  <b>{a.name}</b>
                  <small>
                    {a.location} · {a.activity}
                  </small>
                </div>
                <span className="tag">
                  {a.universe_id === 'platform-universe' ? '平台宇宙' : '个人宇宙'}
                </span>
              </div>
            ))}
            <h3>任务状态</h3>
            {detail.tasks?.length ? (
              detail.tasks.map((t) => (
                <div className="resident" key={t.id}>
                  <FileText size={18} />
                  <span className="mono">{t.id.slice(0, 12)}</span>
                  <span className={'status ' + t.state}>{taskLabels[t.state]}</span>
                </div>
              ))
            ) : (
              <p className="muted-small">当前没有任务记录。</p>
            )}
            {detail.role !== 'admin' && (
              <form className="status-form" onSubmit={toggleStatus}>
                <label>
                  管理操作原因
                  <input name="reason" required minLength={3} placeholder="填写本次操作的原因" />
                </label>
                <button
                  className={'button ' + (detail.status === 'active' ? 'danger-button' : 'primary')}
                  disabled={busy}
                >
                  {detail.status === 'active' ? '停用账户' : '恢复账户'}
                </button>
              </form>
            )}
          </div>
        </Modal>
      )}
    </div>
  )
}
