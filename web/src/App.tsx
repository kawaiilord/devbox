import { Avatar, Modal, WorldScene } from './components'
import Login from './pages/Login'
import Admin from './pages/Admin'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import {
  ArrowDownToLine,
  ArrowLeft,
  ArrowRight,
  Bell,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  Clock3,
  Compass,
  ExternalLink,
  FileText,
  Globe2,
  Heart,
  Leaf,
  LoaderCircle,
  LockKeyhole,
  LogOut,
  MapPin,
  MessageCircle,
  MoreHorizontal,
  Orbit,
  Phone,
  Plus,
  Search,
  Send,
  Settings2,
  ShieldCheck,
  Sparkles,
  SquarePen,
  Users,
  Video,
  WandSparkles,
  X,
} from 'lucide-react'
import { api, post, formatDate, formatTime, taskLabels } from './api'
import type {
  Agent,
  Artifact,
  Memory,
  Message,
  Post,
  Provider,
  Task,
  User,
  World,
  WorldEvent,
} from './api'
import './styles.css'
import './readability.css'

type Tab = 'chat' | 'friends' | 'discover' | 'me'
type ModalState = 'agent' | 'provider' | 'task' | 'memory' | 'post' | null

function App() {
  const [user, setUser] = useState<User | null>(null),
    [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<Tab>('chat'),
    [discovery, setDiscovery] = useState<'moments' | 'world'>('moments')
  const [agents, setAgents] = useState<Agent[]>([]),
    [worlds, setWorlds] = useState<World[]>([]),
    [profiles, setProfiles] = useState<Provider[]>([]),
    [posts, setPosts] = useState<Post[]>([]),
    [tasks, setTasks] = useState<Task[]>([])
  const [selected, setSelected] = useState(''),
    [messages, setMessages] = useState<Message[]>([]),
    [memories, setMemories] = useState<Memory[]>([])
  const [worldId, setWorldId] = useState(''),
    [events, setEvents] = useState<WorldEvent[]>([])
  const [modal, setModal] = useState<ModalState>(null),
    [toast, setToast] = useState(''),
    [busy, setBusy] = useState(false),
    [sending, setSending] = useState(false)
  const [draft, setDraft] = useState(''),
    [search, setSearch] = useState(''),
    [profileOpen, setProfileOpen] = useState(false),
    [taskDetail, setTaskDetail] = useState<Task | null>(null),
    [preview, setPreview] = useState<Artifact | null>(null)
  const [mobileConversation, setMobileConversation] = useState(false)
  const bottom = useRef<HTMLDivElement>(null)
  const selectedRef = useRef(selected)
  selectedRef.current = selected
  const worldRef = useRef(worldId)
  worldRef.current = worldId
  const agent = agents.find((a) => a.id === selected)
  const world = worlds.find((w) => w.id === worldId)
  const isAdminPath = location.pathname.startsWith('/admin')

  const notify = useCallback((msg: string) => setToast(msg), [])
  const refresh = useCallback(async () => {
    const [a, w, p, f, t] = await Promise.all([
      api<Agent[]>('/agents'),
      api<World[]>('/universes'),
      api<Provider[]>('/providers'),
      api<Post[]>('/posts'),
      api<Task[]>('/tasks'),
    ])
    setAgents(a)
    setWorlds(w)
    setProfiles(p)
    setPosts(f)
    setTasks(t)
    setSelected((current) => (a.some((x) => x.id === current) ? current : (a[0]?.id ?? '')))
    setWorldId((current) =>
      w.some((x) => x.id === current) ? current : (w.find((x) => x.kind === 'private')?.id ?? ''),
    )
  }, [])
  const loadConversation = useCallback(async (id: string) => {
    const [m, n] = await Promise.all([
      api<Message[]>('/agents/' + id + '/messages'),
      api<Memory[]>('/agents/' + id + '/memories'),
    ])
    if (selectedRef.current === id) {
      setMessages(m)
      setMemories(n)
    }
  }, [])
  const run = useCallback(
    async (fn: () => Promise<void>) => {
      setBusy(true)
      try {
        await fn()
      } catch (e) {
        notify((e as Error).message)
      } finally {
        setBusy(false)
      }
    },
    [notify],
  )
  useEffect(() => {
    api<User>('/me')
      .then(setUser)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])
  useEffect(() => {
    if (!user) return
    refresh().catch((e) => notify(e.message))
    const interval = setInterval(() => {
      if (document.visibilityState === 'visible') {
        post('/heartbeat').catch(() => {})
        refresh().catch(() => {})
      }
    }, 25000)
    return () => clearInterval(interval)
  }, [user, refresh, notify])
  useEffect(() => {
    if (selected) loadConversation(selected).catch((e) => notify(e.message))
  }, [selected, loadConversation, notify])
  useEffect(() => {
    let active = true
    if (worldId)
      api<WorldEvent[]>('/universes/' + worldId + '/events')
        .then((e) => {
          if (active) setEvents(e)
        })
        .catch((e) => {
          if (active) notify(e.message)
        })
    return () => {
      active = false
    }
  }, [worldId, posts.length, notify])
  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length, sending])
  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 6500)
    return () => clearTimeout(timer)
  }, [toast])
  useEffect(() => {
    if (!taskDetail || !['queued', 'running'].includes(taskDetail.state)) return
    const timer = setInterval(() => {
      api<Task>('/tasks/' + taskDetail.id)
        .then(setTaskDetail)
        .catch(() => {})
    }, 2500)
    return () => clearInterval(timer)
  }, [taskDetail])

  async function send() {
    if (!agent || !draft.trim() || sending) return
    const content = draft.trim()
    setDraft('')
    setSending(true)
    try {
      await post('/agents/' + agent.id + '/messages', { content, client_id: crypto.randomUUID() })
      await loadConversation(agent.id)
    } catch (e) {
      notify((e as Error).message)
      setDraft(content)
      await loadConversation(agent.id)
    } finally {
      setSending(false)
    }
  }
  async function move(a: Agent, target: string) {
    await run(async () => {
      await post('/agents/' + a.id + '/migrate', {
        universe_id: target,
        expected_version: a.version,
        operation_id: crypto.randomUUID(),
      })
      await refresh()
      notify(a.name + ' 已进入新的宇宙')
    })
  }
  async function chooseProvider(id: string) {
    if (!agent) return
    await run(async () => {
      await api('/agents/' + agent.id, {
        method: 'PATCH',
        body: JSON.stringify({ provider_id: id || null }),
      })
      await refresh()
      notify('对话模型已更新')
    })
  }
  async function submitModal(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const data = Object.fromEntries(new FormData(e.currentTarget).entries())
    await run(async () => {
      if (modal === 'agent') {
        const a = await post<Agent>('/agents', data)
        await refresh()
        setSelected(a.id)
        notify('新的朋友已加入你的小宇宙')
      }
      if (modal === 'provider') {
        const p = await post<Provider>('/providers', data)
        if (agent)
          await api('/agents/' + agent.id, {
            method: 'PATCH',
            body: JSON.stringify({ provider_id: p.id }),
          })
        await refresh()
        notify('模型已保存' + (agent ? '，并为 ' + agent.name + ' 启用' : ''))
      }
      if (modal === 'memory' && agent) {
        await post('/agents/' + agent.id + '/memories', data)
        await loadConversation(agent.id)
        notify('这件事已经记下')
      }
      if (modal === 'task') {
        const t = await post<Task>('/tasks', data)
        await refresh()
        setTaskDetail(await api<Task>('/tasks/' + t.id))
      }
      if (modal === 'post') {
        await post('/posts', data)
        await refresh()
        notify('动态已发布')
      }
      setModal(null)
    })
  }
  const closeModal = useCallback(() => setModal(null), [])
  const closeProfile = useCallback(() => setProfileOpen(false), [])
  const closeTask = useCallback(() => setTaskDetail(null), [])
  const closePreview = useCallback(() => setPreview(null), [])
  if (loading)
    return (
      <div className="page-loading">
        <Orbit className="spin" size={30} />
        <span>正在打开你的小宇宙</span>
      </div>
    )
  if (!user) return <Login onLogin={setUser} />
  if (isAdminPath)
    return (
      <Admin
        user={user}
        onLogout={() =>
          run(async () => {
            await post('/auth/logout')
            location.href = '/'
          })
        }
      />
    )

  const nav = [
    { key: 'chat' as Tab, icon: MessageCircle, name: '聊天' },
    { key: 'friends' as Tab, icon: Users, name: '好友' },
    { key: 'discover' as Tab, icon: Compass, name: '发现' },
    { key: 'me' as Tab, icon: Settings2, name: '我的' },
  ]
  return (
    <div className="app">
      <aside className="rail">
        <a className="brand-icon" href="/" title="邻宇">
          <Orbit size={29} />
        </a>
        <nav>
          {nav.map(({ key, icon: Icon, name }) => (
            <button
              key={key}
              className={'rail-link ' + (tab === key ? 'active' : '')}
              onClick={() => {
                setTab(key)
                setMobileConversation(false)
              }}
              aria-label={name}
            >
              <Icon size={22} />
              <span>{name}</span>
              {key === 'discover' && posts.length > 0 && <i />}
            </button>
          ))}
        </nav>
        <div className="rail-bottom">
          {user.role === 'admin' && (
            <a className="rail-link admin-link" href="/admin" title="管理后台">
              <ShieldCheck size={21} />
              <span>管理</span>
            </a>
          )}
          <button className="my-avatar" onClick={() => setTab('me')} title={user.name}>
            {user.name.slice(0, 1)}
          </button>
        </div>
      </aside>
      <div className="app-body">
        <header className="topbar">
          <div className="wordmark">
            <b>邻宇</b>
            <span>LINYU</span>
            <i />
            <small>把生活，聊成故事。</small>
          </div>
          <div className="topbar-right">
            <span className="world-pill">
              <span className="live-dot" />
              我的小宇宙
            </span>
            <button
              className="icon-button"
              aria-label="通知"
              onClick={() => notify('新的世界动态会出现在「发现」中')}
            >
              <Bell size={18} />
            </button>
            <span className="user-mini">{user.name}</span>
          </div>
        </header>
        <main
          className={
            'main-content ' +
            (tab === 'chat' ? 'chat-layout ' : '') +
            (mobileConversation ? 'mobile-open' : '')
          }
        >
          {tab === 'chat' && (
            <>
              <section className="conversation-list">
                <div className="pane-heading">
                  <h1>
                    聊天<span>{agents.length}</span>
                  </h1>
                  <button
                    className="icon-button"
                    aria-label="添加好友"
                    onClick={() => setModal('agent')}
                  >
                    <SquarePen size={20} />
                  </button>
                </div>
                <div className="search-field">
                  <Search size={16} />
                  <input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="搜索朋友"
                  />
                </div>
                <div className="section-eyebrow">最近联系</div>
                <div className="conversation-items">
                  {agents
                    .filter((a) => a.name.includes(search))
                    .map((a) => (
                      <button
                        key={a.id}
                        className={'conversation ' + (selected === a.id ? 'selected' : '')}
                        onClick={() => {
                          setSelected(a.id)
                          setMobileConversation(true)
                        }}
                      >
                        <Avatar name={a.name} color={a.color} />
                        <div>
                          <div className="conversation-title">
                            <b>{a.name}</b>
                            <span>{a.location}</span>
                          </div>
                          <p>{a.activity}</p>
                        </div>
                      </button>
                    ))}
                </div>
                <div className="sidebar-foot">
                  <Leaf size={17} />
                  <p>
                    他们也有自己的生活。
                    <br />
                    <span>下次聊天，会有新的故事。</span>
                  </p>
                </div>
              </section>
              {agent ? (
                <>
                  <section className="chat-panel">
                    <div className="chat-header">
                      <button
                        className="icon-button mobile-back"
                        aria-label="返回聊天列表"
                        onClick={() => setMobileConversation(false)}
                      >
                        <ArrowLeft size={20} />
                      </button>
                      <Avatar name={agent.name} color={agent.color} size="small" />
                      <button className="chat-person" onClick={() => setProfileOpen(true)}>
                        <b>
                          {agent.name}
                          <ChevronDown size={14} />
                        </b>
                        <span>
                          <span className="live-dot" />
                          {agent.location} · {agent.activity}
                        </span>
                      </button>
                      <div className="chat-actions">
                        <button
                          className="icon-button"
                          disabled
                          title="实时语音将在后续版本接入"
                          aria-label="语音通话尚未接入"
                        >
                          <Phone size={20} />
                        </button>
                        <button
                          className="icon-button"
                          disabled
                          title="实时视频将在后续版本接入"
                          aria-label="视频通话尚未接入"
                        >
                          <Video size={21} />
                        </button>
                        <button
                          className="icon-button"
                          aria-label="好友资料"
                          onClick={() => setProfileOpen(true)}
                        >
                          <MoreHorizontal size={21} />
                        </button>
                      </div>
                    </div>
                    <div className="chat-messages">
                      <div className="date-divider">
                        <span>
                          {new Date().toLocaleDateString('zh-CN', {
                            month: 'long',
                            day: 'numeric',
                          })}{' '}
                          · 一个新的日常
                        </span>
                      </div>
                      <div className="life-update">
                        <Leaf size={14} />
                        <span>
                          {agent.name}正在{agent.location}
                        </span>
                        <button
                          onClick={() => {
                            setTab('discover')
                            setDiscovery('world')
                            setWorldId(agent.universe_id)
                          }}
                        >
                          去看看
                          <ChevronRight size={13} />
                        </button>
                      </div>
                      {messages.length === 0 && (
                        <div className="conversation-empty">
                          <span className="empty-orbit">
                            <Orbit size={44} />
                          </span>
                          <h2>今天，从一句你好开始。</h2>
                          <p>
                            聊聊日常，分享灵感，
                            <br />
                            也可以请{agent.name}帮你完成一个想法。
                          </p>
                          {!agent.provider_id && (
                            <button
                              className="button subtle"
                              onClick={() =>
                                profiles.length ? setProfileOpen(true) : setModal('provider')
                              }
                            >
                              为{agent.name}选择对话模型
                              <ArrowRight size={15} />
                            </button>
                          )}
                        </div>
                      )}
                      {messages.map((m) => (
                        <div
                          key={m.id}
                          className={'message ' + (m.role === 'user' ? 'outgoing' : 'incoming')}
                        >
                          {m.role !== 'user' && (
                            <Avatar name={agent.name} color={agent.color} size="small" />
                          )}
                          <div>
                            <div className="bubble">{m.content}</div>
                            <span className="message-time">
                              {formatTime(m.created_at)} {m.state === 'failed' ? '· 发送失败' : ''}
                            </span>
                          </div>
                          {m.role === 'user' && (
                            <span className="self-avatar">{user.name.slice(0, 1)}</span>
                          )}
                        </div>
                      ))}
                      {sending && (
                        <div className="typing">
                          <Avatar name={agent.name} color={agent.color} size="small" />
                          <span>
                            <i />
                            <i />
                            <i />
                          </span>
                        </div>
                      )}
                      <div ref={bottom} />
                    </div>
                    <div className="composer">
                      <div className="composer-tools">
                        <button title="委托任务" onClick={() => setModal('task')}>
                          <WandSparkles size={17} />
                          委托任务
                        </button>
                        <button title="记住一件事" onClick={() => setModal('memory')}>
                          <BookOpen size={17} />
                          记住一件事
                        </button>
                        <span>
                          {agent.provider_id
                            ? (profiles.find((p) => p.id === agent.provider_id)?.name ??
                              '模型已连接')
                            : '尚未选择模型'}
                        </span>
                      </div>
                      <textarea
                        aria-label="聊天内容"
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                            e.preventDefault()
                            void send()
                          }
                        }}
                        placeholder={'和' + agent.name + '说点什么…'}
                        rows={2}
                      />
                      <div className="composer-bottom">
                        <span>Enter 发送 · Shift + Enter 换行</span>
                        <button
                          className="send-button"
                          aria-label="发送消息"
                          onClick={() => void send()}
                          disabled={sending || !draft.trim()}
                        >
                          {sending ? (
                            <LoaderCircle className="spin" size={17} />
                          ) : (
                            <>
                              发送
                              <Send size={15} />
                            </>
                          )}
                        </button>
                      </div>
                    </div>
                  </section>
                  <aside className="context-panel">
                    <div className="context-label">
                      <Sparkles size={15} />
                      朋友的此刻
                    </div>
                    <div
                      className="profile-cover"
                      style={{ '--cover': agent.color } as React.CSSProperties}
                    >
                      <span className="cover-sun" />
                      <span className="cover-hill" />
                      <Avatar name={agent.name} color={agent.color} size="large" />
                    </div>
                    <div className="context-profile">
                      <h2>{agent.name}</h2>
                      <p>{agent.subtitle}</p>
                    </div>
                    <div className="now-card">
                      <span className="eyebrow">正在发生</span>
                      <h3>
                        <MapPin size={15} />
                        {agent.location}
                      </h3>
                      <p>{agent.activity}</p>
                      <span className="world-label">
                        {worlds.find((w) => w.id === agent.universe_id)?.name}
                      </span>
                    </div>
                    <div className="context-section">
                      <div>
                        <h3>共同记忆</h3>
                        <button className="text-button" onClick={() => setModal('memory')}>
                          <Plus size={15} />
                        </button>
                      </div>
                      {memories.length ? (
                        memories.slice(0, 2).map((m) => (
                          <p className="memory-snippet" key={m.id}>
                            <BookOpen size={14} />
                            {m.content}
                          </p>
                        ))
                      ) : (
                        <p className="muted-small">重要的小事，会在这里慢慢积累。</p>
                      )}
                    </div>
                    <div className="context-section">
                      <div>
                        <h3>一起做点什么</h3>
                      </div>
                      <button className="task-teaser" onClick={() => setModal('task')}>
                        <span>
                          <WandSparkles size={19} />
                        </span>
                        <div>
                          <b>把一个想法交给我</b>
                          <small>文档、数据、网页、程序…</small>
                        </div>
                        <ChevronRight size={16} />
                      </button>
                    </div>
                    <div className="context-footer">
                      <Leaf size={14} />
                      让关系，随时间生长。
                    </div>
                  </aside>
                </>
              ) : (
                <div className="empty-state">
                  创建一位好友，开启新的故事。
                  <button className="button primary" onClick={() => setModal('agent')}>
                    添加好友
                  </button>
                </div>
              )}
            </>
          )}
          {tab === 'friends' && (
            <div className="page">
              <div className="page-heading">
                <div>
                  <span className="eyebrow">YOUR PEOPLE, YOUR STORIES</span>
                  <h1>朋友们</h1>
                  <p>每个人都有自己的生活，也有与你共同的故事。</p>
                </div>
                <button className="button primary" onClick={() => setModal('agent')}>
                  <Plus size={17} />
                  添加 AI 好友
                </button>
              </div>
              <div className="friends-grid">
                {agents.map((a) => (
                  <article className="friend-card" key={a.id}>
                    <div className="friend-cover" style={{ background: a.color + '20' }}>
                      <span className="friend-orbit" />
                      <Avatar name={a.name} color={a.color} size="large" />
                      <span className="friend-world">
                        {worlds.find((w) => w.id === a.universe_id)?.name}
                      </span>
                    </div>
                    <div className="friend-content">
                      <h2>{a.name}</h2>
                      <p>{a.subtitle}</p>
                      <span className="activity-line">
                        <span className="live-dot" />
                        {a.activity}
                      </span>
                      <div className="friend-buttons">
                        <button
                          className="button subtle"
                          onClick={() => {
                            setSelected(a.id)
                            setTab('chat')
                            setMobileConversation(true)
                          }}
                        >
                          <MessageCircle size={16} />
                          聊一聊
                        </button>
                        <button
                          className="icon-button"
                          aria-label={'管理' + a.name}
                          onClick={() => {
                            setSelected(a.id)
                            setProfileOpen(true)
                          }}
                        >
                          <Settings2 size={17} />
                        </button>
                      </div>
                    </div>
                  </article>
                ))}
                <button className="friend-add" onClick={() => setModal('agent')}>
                  <span>
                    <Plus size={25} />
                  </span>
                  <b>认识一位新朋友</b>
                  <small>赋予名字、人设和自己的生活</small>
                </button>
              </div>
            </div>
          )}
          {tab === 'discover' && (
            <div className="page discovery">
              <div className="page-heading">
                <div>
                  <span className="eyebrow">LIFE BEYOND THE CONVERSATION</span>
                  <h1>发现生活的另一面</h1>
                  <p>你不在的时候，故事也在继续。</p>
                </div>
                <button className="button primary" onClick={() => setModal('post')}>
                  <SquarePen size={17} />
                  分享此刻
                </button>
              </div>
              <div className="segmented">
                <button
                  className={discovery === 'moments' ? 'active' : ''}
                  onClick={() => setDiscovery('moments')}
                >
                  <Leaf size={16} />
                  朋友圈
                </button>
                <button
                  className={discovery === 'world' ? 'active' : ''}
                  onClick={() => setDiscovery('world')}
                >
                  <Globe2 size={16} />
                  我的宇宙
                </button>
              </div>
              {discovery === 'moments' ? (
                <div className="feed-layout">
                  <div className="feed">
                    <div className="feed-cover">
                      <div>
                        <span className="eyebrow">MOMENTS IN OUR LITTLE WORLD</span>
                        <h2>
                          把平凡的日子，
                          <br />
                          过成喜欢的样子。
                        </h2>
                        <p>来自你和朋友们的生活片段</p>
                      </div>
                      <div className="feed-decoration">
                        <Leaf size={80} strokeWidth={1} />
                        <span />
                      </div>
                    </div>
                    {posts.length ? (
                      posts.map((p) => (
                        <article className="post-card" key={p.id}>
                          <Avatar name={p.author_name} color={p.author_color} />
                          <div className="post-body">
                            <div className="post-author">
                              <b>{p.author_name}</b>
                              <span>{worlds.find((w) => w.id === p.universe_id)?.name}</span>
                            </div>
                            <p>{p.content}</p>
                            {p.event_id && (
                              <div className="post-event">
                                <Leaf size={18} />
                                <span>
                                  一次共同经历<small>来自宇宙生活事件</small>
                                </span>
                              </div>
                            )}
                            <div className="post-meta">
                              <span>{formatDate(p.created_at)}</span>
                              {p.agent_id && agents.some((a) => a.id === p.agent_id) && (
                                <button
                                  onClick={() => {
                                    setSelected(p.agent_id!)
                                    setTab('chat')
                                    setMobileConversation(true)
                                  }}
                                >
                                  <MessageCircle size={14} />
                                  聊聊这件事
                                </button>
                              )}
                            </div>
                          </div>
                        </article>
                      ))
                    ) : (
                      <div className="empty-state">
                        <Leaf size={30} />
                        <h3>第一段故事，正在路上</h3>
                        <p>去小宇宙开启一次活动，或者分享你的此刻。</p>
                        <button className="button subtle" onClick={() => setDiscovery('world')}>
                          看看我的宇宙
                          <ArrowRight size={15} />
                        </button>
                      </div>
                    )}
                  </div>
                  <aside className="feed-aside">
                    <h3>你的小宇宙</h3>
                    <WorldScene compact />
                    <p>{agents.length} 位朋友，在各自的生活中与你相遇。</p>
                    <button className="button subtle" onClick={() => setDiscovery('world')}>
                      进入宇宙
                      <ArrowRight size={15} />
                    </button>
                    <div className="quiet-note">
                      <Heart size={20} />
                      <p>
                        有些小事，
                        <br />
                        因为有人分享而变得重要。
                      </p>
                    </div>
                  </aside>
                </div>
              ) : (
                <>
                  <div className="world-selectors">
                    {worlds.map((w) => (
                      <button
                        key={w.id}
                        className={worldId === w.id ? 'selected' : ''}
                        onClick={() => setWorldId(w.id)}
                      >
                        {w.kind === 'private' ? <LockKeyhole size={19} /> : <Globe2 size={19} />}
                        <div>
                          <b>{w.name}</b>
                          <span>
                            {w.kind === 'private' ? '你和朋友们的专属生活' : '与更多 AI 自由相遇'}
                          </span>
                        </div>
                        <span className="population">{w.population} 位居民</span>
                      </button>
                    ))}
                  </div>
                  {world && (
                    <div className="universe-layout">
                      <div>
                        <div className="world-map-card">
                          <div className="world-map-heading">
                            <div>
                              <span className="eyebrow">
                                {world.kind === 'private' ? 'PRIVATE UNIVERSE' : 'SHARED UNIVERSE'}
                              </span>
                              <h2>{world.name}</h2>
                            </div>
                            <span className="tag">
                              <span className="live-dot" />
                              运行中
                            </span>
                          </div>
                          <WorldScene />
                          <div className="world-map-caption">
                            <span>{world.description}</span>
                            {(world.kind === 'private' || user.role === 'admin') && (
                              <button
                                className="button primary small"
                                disabled={busy}
                                onClick={() =>
                                  run(async () => {
                                    await post('/universes/' + world.id + '/advance')
                                    await refresh()
                                    const nextEvents = await api<WorldEvent[]>(
                                      '/universes/' + world.id + '/events',
                                    )
                                    if (worldRef.current === world.id) setEvents(nextEvents)
                                    notify('新的生活事件已经发生')
                                  })
                                }
                              >
                                <Sparkles size={15} />
                                开启一次活动
                              </button>
                            )}
                          </div>
                        </div>
                        <div className="residents-heading">
                          <h3>在这里的朋友</h3>
                          <span>切换宇宙时，与你的关系会延续</span>
                        </div>
                        <div className="resident-list">
                          {agents
                            .filter((a) => a.universe_id === world.id)
                            .map((a) => (
                              <div className="resident" key={a.id}>
                                <Avatar name={a.name} color={a.color} size="small" />
                                <div>
                                  <b>{a.name}</b>
                                  <small>
                                    {a.location} · {a.activity}
                                  </small>
                                </div>
                                <select
                                  aria-label={'迁移' + a.name}
                                  value={a.universe_id}
                                  disabled={busy}
                                  onChange={(e) => void move(a, e.target.value)}
                                >
                                  {worlds.map((w) => (
                                    <option value={w.id} key={w.id}>
                                      {w.name}
                                    </option>
                                  ))}
                                </select>
                              </div>
                            ))}
                          {agents.every((a) => a.universe_id !== world.id) && (
                            <p className="empty-inline">
                              你的好友还没有迁入这里。可在好友资料中选择所属宇宙。
                            </p>
                          )}
                        </div>
                      </div>
                      <aside className="timeline-card">
                        <h3>
                          <Clock3 size={17} />
                          生活时间线
                        </h3>
                        {events.length ? (
                          events.map((e) => (
                            <div className="timeline-event" key={e.id}>
                              <span />
                              <small>{formatDate(e.created_at)}</small>
                              <b>{e.title}</b>
                              <p>{e.content}</p>
                            </div>
                          ))
                        ) : (
                          <p className="muted-small">等待这里的第一场相遇。</p>
                        )}
                      </aside>
                    </div>
                  )}
                </>
              )}
            </div>
          )}
          {tab === 'me' && (
            <div className="page settings-page">
              <div className="page-heading">
                <div>
                  <span className="eyebrow">MAKE THIS WORLD YOURS</span>
                  <h1>我的</h1>
                  <p>按照你的方式，与世界相处。</p>
                </div>
                <button
                  className="button ghost"
                  onClick={() =>
                    run(async () => {
                      await post('/auth/logout')
                      location.href = '/'
                    })
                  }
                >
                  <LogOut size={16} />
                  退出登录
                </button>
              </div>
              <div className="account-card">
                <span className="account-avatar">{user.name.slice(0, 1)}</span>
                <div>
                  <h2>{user.name}</h2>
                  <p>{user.email}</p>
                </div>
                <span className="tag">{user.role === 'admin' ? '平台管理员' : '宇宙居民'}</span>
                {user.role === 'admin' && (
                  <a className="button subtle" href="/admin">
                    <ShieldCheck size={16} />
                    管理后台
                    <ExternalLink size={14} />
                  </a>
                )}
              </div>
              <section className="settings-section">
                <div className="section-heading">
                  <div>
                    <h2>对话模型</h2>
                    <p>为不同好友选择适合的模型，身份与记忆会保留。</p>
                  </div>
                  <button className="button subtle" onClick={() => setModal('provider')}>
                    <Plus size={16} />
                    添加模型
                  </button>
                </div>
                {profiles.length ? (
                  <div className="provider-list">
                    {profiles.map((p) => (
                      <div className="provider-card" key={p.id}>
                        <span className="provider-symbol">
                          <Sparkles size={20} />
                        </span>
                        <div>
                          <b>{p.name}</b>
                          <p>{p.model}</p>
                          <small>{p.base_url}</small>
                        </div>
                        <span className="tag">
                          <LockKeyhole size={12} />
                          密钥已保存
                        </span>
                        <button
                          className="icon-button"
                          aria-label={'删除模型' + p.name}
                          onClick={() =>
                            run(async () => {
                              await api('/providers/' + p.id, { method: 'DELETE' })
                              await refresh()
                              notify('模型配置已删除')
                            })
                          }
                        >
                          <X size={17} />
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="settings-empty">
                    <Sparkles size={24} />
                    <div>
                      <b>给朋友连接一个对话模型</b>
                      <p>支持兼容接口和 Anthropic 原生接口，可保存多个配置。</p>
                    </div>
                    <button className="button primary" onClick={() => setModal('provider')}>
                      开始配置
                      <ArrowRight size={15} />
                    </button>
                  </div>
                )}
              </section>
              <section className="settings-section">
                <div className="section-heading">
                  <div>
                    <h2>交给朋友的事</h2>
                    <p>从一个想法开始，查看每一步和实际交付。</p>
                  </div>
                  <button className="button subtle" onClick={() => setModal('task')}>
                    <Plus size={16} />
                    创建任务
                  </button>
                </div>
                {tasks.length ? (
                  <div className="task-list">
                    {tasks.map((t) => (
                      <button
                        className="task-row"
                        key={t.id}
                        onClick={() =>
                          run(async () => setTaskDetail(await api<Task>('/tasks/' + t.id)))
                        }
                      >
                        <span className="task-file">
                          <FileText size={20} />
                        </span>
                        <div>
                          <b>{t.goal}</b>
                          <small>
                            {agents.find((a) => a.id === t.agent_id)?.name} ·{' '}
                            {formatDate(t.created_at)}
                          </small>
                        </div>
                        <span className={'status ' + t.state}>
                          {taskLabels[t.state] ?? t.state}
                        </span>
                        <ChevronRight size={17} />
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="empty-inline">
                    还没有任务。可以请朋友写文档、整理表格，或者生成一个网页。
                  </div>
                )}
              </section>
              <section className="settings-section">
                <div className="section-heading">
                  <div>
                    <h2>共同记忆</h2>
                    <p>你可以添加，也可以让朋友忘记一件事。</p>
                  </div>
                  <select
                    aria-label="选择好友查看记忆"
                    value={selected}
                    onChange={(e) => setSelected(e.target.value)}
                  >
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </select>
                </div>
                {memories.length ? (
                  memories.map((m) => (
                    <div className="memory-row" key={m.id}>
                      <BookOpen size={17} />
                      <div>
                        <p>{m.content}</p>
                        <small>
                          {m.source === 'world_event' ? '世界经历' : '你告诉朋友的事'} ·{' '}
                          {formatDate(m.created_at)}
                        </small>
                      </div>
                      <button
                        className="text-button danger"
                        onClick={() =>
                          run(async () => {
                            await api('/memories/' + m.id, { method: 'DELETE' })
                            await loadConversation(selected)
                            notify('这条记忆已删除')
                          })
                        }
                      >
                        忘记
                      </button>
                    </div>
                  ))
                ) : (
                  <div className="empty-inline">共同记忆会随着相处慢慢积累。</div>
                )}
              </section>
            </div>
          )}
        </main>
        <footer className="app-footer">
          <span>
            <Leaf size={12} />
            邻宇 · 每一个世界，都有你的朋友
          </span>
          <span>FOUNDATION 0.1</span>
        </footer>
      </div>
      {toast && (
        <div className="toast" role="status">
          <CircleHelp size={17} />
          <span>{toast}</span>
          <button aria-label="关闭提示" onClick={() => setToast('')}>
            <X size={15} />
          </button>
        </div>
      )}
      {modal && (
        <Modal
          title={
            {
              agent: '认识一位新朋友',
              provider: '添加对话模型',
              task: '把一个想法交给朋友',
              memory: '记住一件小事',
              post: '分享此刻',
            }[modal]
          }
          onClose={closeModal}
        >
          <form onSubmit={submitModal} className="modal-form">
            {modal === 'agent' && (
              <>
                <p className="muted">从名字和性格开始，让朋友拥有自己的生活。</p>
                <label>
                  名字
                  <input name="name" required maxLength={40} placeholder="朋友的名字" />
                </label>
                <label>
                  身份与兴趣
                  <input
                    name="subtitle"
                    required
                    maxLength={80}
                    placeholder="例如：建筑师，喜欢旧书与城市散步"
                  />
                </label>
                <label>
                  性格与生活背景
                  <textarea
                    name="persona"
                    required
                    rows={4}
                    maxLength={3000}
                    placeholder="描述说话习惯、兴趣、目标，以及你期待的相处方式。"
                  />
                </label>
                <label>
                  角色颜色
                  <input name="color" type="color" defaultValue="#6a8b78" />
                </label>
              </>
            )}
            {modal === 'provider' && (
              <>
                <p className="muted">配置会加密保存。每位好友可以单独选择模型。</p>
                <label>
                  配置名称
                  <input name="name" required placeholder="例如：日常聊天" />
                </label>
                <label>
                  接口类型
                  <select name="protocol">
                    <option value="compatible">兼容 Chat Completions</option>
                    <option value="anthropic">Anthropic Messages</option>
                  </select>
                </label>
                <label>
                  接口地址
                  <input
                    name="base_url"
                    type="url"
                    required
                    placeholder="https://你的服务域名/v1"
                  />
                </label>
                <label>
                  模型 ID
                  <input name="model" required placeholder="服务提供方的准确模型名称" />
                </label>
                <label>
                  API Key
                  <input
                    name="api_key"
                    type="password"
                    required
                    autoComplete="new-password"
                    placeholder="密钥仅用于请求你配置的服务"
                  />
                </label>
              </>
            )}
            {modal === 'task' && (
              <>
                <p className="muted">
                  可以写报告、整理表格、生成网页或程序源码。当前已接入文件读写能力；任务会展示真实执行步骤。
                </p>
                <label>
                  交给哪位朋友
                  <select name="agent_id" defaultValue={selected}>
                    {agents.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  你想完成什么
                  <textarea
                    name="goal"
                    required
                    minLength={2}
                    maxLength={8000}
                    rows={5}
                    placeholder="例如：帮我做一份三天旅行计划，包含每天的行程和预算，交付为 Markdown 和 CSV 文件。"
                  />
                </label>
                <div className="capability-chips">
                  <span>
                    <FileText size={13} />
                    文档
                  </span>
                  <span>表格与数据</span>
                  <span>网页与源码</span>
                </div>
              </>
            )}
            {modal === 'memory' && (
              <>
                <p className="muted">告诉{agent?.name}一件重要的事。</p>
                <label>
                  内容
                  <textarea
                    name="content"
                    required
                    maxLength={2000}
                    rows={4}
                    placeholder="例如：我喜欢简短的回复，周末通常会去散步。"
                  />
                </label>
                <label>
                  记忆类型
                  <select name="category">
                    <option value="preference">习惯与偏好</option>
                    <option value="promise">约定与计划</option>
                    <option value="experience">共同经历</option>
                  </select>
                </label>
              </>
            )}
            {modal === 'post' && (
              <>
                <label>
                  分享什么
                  <textarea
                    name="content"
                    required
                    maxLength={3000}
                    rows={5}
                    placeholder="记录今天的小小瞬间…"
                  />
                </label>
                <label>
                  发布到
                  <select name="universe_id" defaultValue={worldId}>
                    {worlds.map((w) => (
                      <option key={w.id} value={w.id}>
                        {w.name}
                        {w.kind === 'public' ? ' · 公共可见' : ' · 仅你的宇宙'}
                      </option>
                    ))}
                  </select>
                </label>
              </>
            )}
            <div className="modal-actions">
              <button type="button" className="button ghost" onClick={closeModal}>
                取消
              </button>
              <button className="button primary" disabled={busy}>
                {busy ? (
                  <LoaderCircle size={16} className="spin" />
                ) : (
                  <>
                    {modal === 'task' ? '开始任务' : '保存'}
                    <ArrowRight size={15} />
                  </>
                )}
              </button>
            </div>
          </form>
        </Modal>
      )}
      {profileOpen && agent && (
        <Modal title="好友资料" onClose={closeProfile}>
          <div className="profile-modal">
            <Avatar name={agent.name} color={agent.color} size="large" />
            <h2>{agent.name}</h2>
            <p>{agent.subtitle}</p>
            <div className="persona-text">{agent.persona}</div>
            <label>
              对话模型
              <select
                value={agent.provider_id ?? ''}
                disabled={busy}
                onChange={(e) => void chooseProvider(e.target.value)}
              >
                <option value="">尚未选择</option>
                {profiles.map((p) => (
                  <option value={p.id} key={p.id}>
                    {p.name} · {p.model}
                  </option>
                ))}
              </select>
            </label>
            <button
              className="text-button"
              onClick={() => {
                setProfileOpen(false)
                setModal('provider')
              }}
            >
              <Plus size={14} />
              添加模型配置
            </button>
            <label>
              当前宇宙
              <select
                value={agent.universe_id}
                disabled={busy}
                onChange={(e) => void move(agent, e.target.value)}
              >
                {worlds.map((w) => (
                  <option value={w.id} key={w.id}>
                    {w.name}
                  </option>
                ))}
              </select>
            </label>
            <div className="inline-note">
              <BookOpen size={16} />
              迁移会保留人格、声音配置和与你的关系。
            </div>
          </div>
        </Modal>
      )}
      {taskDetail && (
        <Modal title="任务详情" onClose={closeTask} wide>
          <div className="task-detail">
            <div className="task-detail-title">
              <WandSparkles size={22} />
              <h3>{taskDetail.goal}</h3>
            </div>
            <span className={'status ' + taskDetail.state}>{taskLabels[taskDetail.state]}</span>
            <p className="task-result">
              {taskDetail.result || '任务已进入队列，朋友会按步骤完成。'}
            </p>
            <div className="task-steps">
              {taskDetail.steps.map((s) => (
                <div key={s.index}>
                  <span>
                    {s.state === 'succeeded' ? <Check size={15} /> : <CircleHelp size={15} />}
                  </span>
                  <div>
                    <b>
                      步骤 {s.index} · {s.tool}
                    </b>
                    <small>{s.path || String(s.result.error || '步骤已记录')}</small>
                  </div>
                  <time>{formatTime(s.at)}</time>
                </div>
              ))}
            </div>
            {!!taskDetail.artifacts?.length && (
              <>
                <h4>实际交付</h4>
                <div className="artifacts">
                  {taskDetail.artifacts.map((a) => (
                    <div key={a.id}>
                      <FileText size={21} />
                      <b>
                        {a.name} <small>v{a.version}</small>
                      </b>
                      <button
                        className="text-button"
                        onClick={() =>
                          run(async () => setPreview(await api<Artifact>('/artifacts/' + a.id)))
                        }
                      >
                        预览
                      </button>
                      <a href={'/api/artifacts/' + a.id + '/download'} title="下载文件">
                        <ArrowDownToLine size={18} />
                      </a>
                    </div>
                  ))}
                </div>
              </>
            )}
            <div className="modal-actions">
              {['failed', 'cancelled', 'waiting_configuration', 'blocked'].includes(
                taskDetail.state,
              ) && (
                <button
                  className="button subtle"
                  onClick={() =>
                    run(async () => {
                      await post('/tasks/' + taskDetail.id + '/retry')
                      setTaskDetail(await api<Task>('/tasks/' + taskDetail.id))
                      await refresh()
                    })
                  }
                >
                  重新尝试
                </button>
              )}
              {['queued', 'running', 'waiting_configuration', 'blocked'].includes(
                taskDetail.state,
              ) && (
                <button
                  className="button ghost danger"
                  onClick={() =>
                    run(async () => {
                      await post('/tasks/' + taskDetail.id + '/cancel')
                      setTaskDetail(await api<Task>('/tasks/' + taskDetail.id))
                      await refresh()
                    })
                  }
                >
                  取消任务
                </button>
              )}
            </div>
          </div>
        </Modal>
      )}
      {preview && (
        <Modal title={preview.name} onClose={closePreview} wide>
          {preview.name.endsWith('.html') ? (
            <iframe
              title="作品预览"
              className="artifact-frame"
              sandbox="allow-scripts"
              srcDoc={
                '<meta http-equiv="Content-Security-Policy" content="default-src &apos;none&apos;; script-src &apos;unsafe-inline&apos;; style-src &apos;unsafe-inline&apos;; img-src data:; connect-src &apos;none&apos;; form-action &apos;none&apos;">' +
                preview.content
              }
            />
          ) : (
            <pre className="artifact-code">{preview.content}</pre>
          )}
        </Modal>
      )}
    </div>
  )
}

export default App
