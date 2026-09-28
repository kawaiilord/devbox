import { useState } from 'react'
import type { FormEvent } from 'react'
import { Orbit, Leaf, ArrowRight, LoaderCircle, LockKeyhole } from 'lucide-react'
import { WorldScene } from '../components'
import { post } from '../api'
import type { User } from '../api'
export default function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [register, setRegister] = useState(false),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    setError('')
    const data = new FormData(e.currentTarget)
    try {
      onLogin(
        await post<User>('/auth/' + (register ? 'register' : 'login'), {
          email: data.get('email'),
          password: data.get('password'),
          ...(register ? { name: data.get('name') } : {}),
        }),
      )
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <main className="login-page">
      <section className="login-story">
        <div className="wordmark">
          <Orbit size={30} />
          <b>邻宇</b>
          <span>LINYU</span>
        </div>
        <div className="login-intro">
          <span className="eyebrow">A LITTLE WORLD, A REAL CONNECTION.</span>
          <h1>
            每一个世界，
            <br />
            都有你的朋友。
          </h1>
          <p>
            聊聊今天，也分享彼此的生活。
            <br />
            和有自己故事的 AI，一起创造新的可能。
          </p>
          <WorldScene />
        </div>
        <span className="login-footer">生活在继续，故事等你来。</span>
      </section>
      <section className="login-form-wrap">
        <form className="login-form" onSubmit={submit}>
          <span className="small-leaf">
            <Leaf size={24} />
          </span>
          <h2>{register ? '开启你的小宇宙' : '欢迎回到邻宇'}</h2>
          <p>{register ? '从两位示例好友和一个个人宇宙开始。' : '你的朋友和故事，都在这里。'}</p>
          {register && (
            <label>
              你的名字
              <input name="name" required maxLength={40} placeholder="朋友们如何称呼你" />
            </label>
          )}
          <label>
            邮箱
            <input
              name="email"
              type="email"
              autoComplete="email"
              required
              placeholder="you@example.com"
            />
          </label>
          <label>
            密码
            <input
              name="password"
              type="password"
              minLength={register ? 10 : 1}
              maxLength={128}
              autoComplete={register ? 'new-password' : 'current-password'}
              required
              placeholder={register ? '至少 10 位' : '输入你的密码'}
            />
          </label>
          {error && (
            <div className="form-error" role="alert">
              {error}
            </div>
          )}
          <button className="button primary large" disabled={busy}>
            {busy ? (
              <LoaderCircle className="spin" size={18} />
            ) : (
              <>
                {register ? '创建我的宇宙' : '进入邻宇'}
                <ArrowRight size={17} />
              </>
            )}
          </button>
          <button
            type="button"
            className="text-button auth-switch"
            onClick={() => {
              setRegister(!register)
              setError('')
            }}
          >
            {register ? '已经有账号？登录' : '第一次来？创建账号'}
          </button>
          <div className="login-note">
            <LockKeyhole size={13} />
            你的私聊和个人宇宙，按账户独立保存
          </div>
        </form>
      </section>
    </main>
  )
}
