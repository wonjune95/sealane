import { useState } from 'react'
import { ApiError, apiPost } from '../api/client'
import { Icon } from '../components/icons'
import { useTheme } from '../ui/theme'

export function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const { theme, toggle } = useTheme()
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim() || !password) return
    setBusy(true)
    setError('')
    try {
      await apiPost('/api/login', { username: username.trim(), password })
      window.location.reload()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <section className="login-story">
        <div className="relative flex items-center gap-3">
          <img src="/favicon.svg" alt="" className="h-9 w-9" />
          <span className="text-lg font-semibold tracking-tight">Sealane</span>
        </div>
        <div className="relative my-auto py-16">
          <p className="mb-5 text-[11px] font-semibold uppercase tracking-[.2em] text-blue-300">
            Your service mesh, connected.
          </p>
          <h1 className="text-4xl font-semibold leading-[1.4] tracking-tight xl:text-5xl">
            복잡한 연결을,
            <br />
            명확한 흐름으로.
          </h1>
          <p className="mt-5 max-w-md text-sm leading-7 text-slate-300">
            라우팅부터 보안 정책까지.
            <br />
            서비스 메시를 한곳에서 살펴보고 관리하세요.
          </p>
          <MeshIllustration />
          <div className="mt-6 flex flex-wrap gap-x-6 gap-y-3 text-xs text-slate-300">
            <span className="flex items-center gap-2">
              <Icon name="route" className="h-4 w-4 text-blue-300" />
              트래픽 라우팅
            </span>
            <span className="flex items-center gap-2">
              <Icon name="shield" className="h-4 w-4 text-blue-300" />
              보안 정책
            </span>
            <span className="flex items-center gap-2">
              <Icon name="chart" className="h-4 w-4 text-blue-300" />
              텔레메트리
            </span>
          </div>
        </div>
        <p className="relative text-[11px] text-slate-400">Built for Istio &amp; Kubernetes Gateway API</p>
      </section>
      <section className="login-form-side relative">
        <button
          onClick={toggle}
          aria-label={theme === 'dark' ? '라이트 모드' : '다크 모드'}
          title={theme === 'dark' ? '라이트 모드' : '다크 모드'}
          className="icon-button absolute right-5 top-5"
        >
          <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="h-5 w-5" />
        </button>
        <div className="w-full max-w-[360px]">
          <div className="mb-10 flex items-center gap-3 lg:hidden">
            <img src="/favicon.svg" alt="" className="h-9 w-9" />
            <span className="font-semibold text-strong">Sealane</span>
          </div>
          <span className="mb-6 flex h-12 w-12 items-center justify-center rounded-2xl border border-blue-100 bg-blue-50 text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-300">
            <Icon name="lock" className="h-5 w-5" />
          </span>
          <h2 className="text-2xl font-semibold tracking-tight text-strong">워크스페이스에 로그인</h2>
          <p className="mb-8 mt-3 text-sm leading-6 text-muted">
            계정으로 로그인하고 서비스 메시를 관리하세요.
          </p>
          <form onSubmit={submit} className="space-y-5">
            <label className="block">
              <span className="mb-2 block text-xs font-medium text-strong">아이디</span>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoFocus
                autoComplete="username"
                placeholder="아이디를 입력하세요"
                required
                className="login-input"
              />
            </label>
            <label className="block">
              <span className="mb-2 block text-xs font-medium text-strong">비밀번호</span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                placeholder="비밀번호를 입력하세요"
                required
                className="login-input"
              />
            </label>
            {error && (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-xs leading-5 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
              >
                <Icon name="warning" className="mt-0.5 h-4 w-4 shrink-0" />
                {error}
              </div>
            )}
            <button
              type="submit"
              disabled={busy || !username.trim() || !password}
              className="btn-primary !mt-6 min-h-11 w-full"
            >
              {busy ? '로그인 중…' : '로그인'}
              <Icon name={busy ? 'refresh' : 'arrow'} className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
            </button>
          </form>
          <div className="mt-8 border-t border-base pt-5">
            <p className="text-center text-xs leading-6 text-muted">
              계정이 필요하거나 로그인이 어려우신가요?
              <br />
              워크스페이스 관리자에게 문의하세요.
            </p>
          </div>
        </div>
      </section>
    </div>
  )
}

function MeshIllustration() {
  return (
    <svg viewBox="0 0 480 180" className="mt-10 w-full max-w-lg" fill="none" aria-hidden="true">
      <path d="M64 90H188M236 90h66M188 90V38h150M188 90v52h150" stroke="#385378" strokeWidth="1.5" />
      <path d="M64 90H188M236 90h66" stroke="#60a5fa" strokeWidth="1.5" />
      <rect x="16" y="66" width="48" height="48" rx="12" fill="#172d4c" stroke="#385378" />
      <path d="M29 90h22m-7-7 7 7-7 7" stroke="#93c5fd" strokeWidth="1.5" />
      <rect x="188" y="66" width="48" height="48" rx="12" fill="#2563eb" />
      <path d="M201 80h22v20h-22zM201 80l22 20" stroke="white" strokeWidth="1.5" />
      <rect x="338" y="18" width="108" height="40" rx="10" fill="#172d4c" stroke="#385378" />
      <rect x="302" y="70" width="144" height="40" rx="10" fill="#172d4c" stroke="#60a5fa" />
      <rect x="338" y="122" width="108" height="40" rx="10" fill="#172d4c" stroke="#385378" />
      <circle cx="356" cy="38" r="3" fill="#60a5fa" />
      <circle cx="320" cy="90" r="3" fill="#34d399" />
      <circle cx="356" cy="142" r="3" fill="#60a5fa" />
      <text x="368" y="42" fontSize="11" fill="#cbd5e1">
        Service A
      </text>
      <text x="332" y="94" fontSize="11" fill="#e2e8f0">
        Service B
      </text>
      <text x="368" y="146" fontSize="11" fill="#cbd5e1">
        Service C
      </text>
      <text x="40" y="137" textAnchor="middle" fontSize="10" fill="#94a3b8">
        Traffic
      </text>
      <text x="212" y="137" textAnchor="middle" fontSize="10" fill="#94a3b8">
        Gateway
      </text>
    </svg>
  )
}
