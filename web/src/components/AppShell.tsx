import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity, BookOpen, ChevronDown, CircleUserRound, Gauge, Info, KeyRound, LayoutDashboard,
  Images, LayoutTemplate, LifeBuoy, LogOut, Menu, PanelLeftClose, PanelLeftOpen, Plus, Search, Settings2, Sparkles, Users, X,
} from 'lucide-react'
import { useAuth } from '../auth/AuthContext'
import { api } from '../api/client'
import { BrandMark, useBrand } from '../branding/BrandContext'
import { Link, navigate, useLocation } from '../router'

interface NavItem { label: string; to: string; icon: ReactNode; admin?: boolean }

const workspaceNav: NavItem[] = [
  { label: '홈', to: '/dashboard', icon: <LayoutDashboard size={18} /> },
  { label: '프레젠테이션', to: '/presentations', icon: <BookOpen size={18} /> },
  { label: '템플릿', to: '/templates', icon: <LayoutTemplate size={18} /> },
  { label: '내 이미지', to: '/images', icon: <Images size={18} /> },
  { label: '새로 만들기', to: '/create', icon: <Sparkles size={18} /> },
]

const settingsNav: NavItem[] = [
  { label: '사용 가이드', to: '/guide', icon: <LifeBuoy size={18} /> },
  { label: '개인화', to: '/profile', icon: <CircleUserRound size={18} /> },
  { label: 'API 키', to: '/api-keys', icon: <KeyRound size={18} /> },
]

const adminNav: NavItem[] = [
  { label: '관리자 개요', to: '/admin', icon: <Gauge size={18} />, admin: true },
  { label: '서비스 설정', to: '/admin/settings', icon: <Settings2 size={18} />, admin: true },
  { label: '사용자', to: '/admin/users', icon: <Users size={18} />, admin: true },
  { label: '오류 · 인시던트', to: '/admin/errors', icon: <Activity size={18} />, admin: true },
]

/**
 * versionLabel reads the build the way a person would say it. A build made
 * outside a release reports itself as "dev", and "vdev" is not a version.
 */
function versionLabel(version: string) {
  if (!version) return '버전 확인 중…'
  if (version === 'dev') return '개발 빌드'
  return /^\d/.test(version) ? `v${version}` : version
}

function PtiumLogo({ compact = false }: { compact?: boolean }) {
  const { productName } = useBrand()
  return (
    <Link to="/dashboard" className={`brand ${compact ? 'compact' : ''}`} ariaLabel={`${productName} 홈`}>
      <BrandMark />
      {!compact && <span>{productName}</span>}
    </Link>
  )
}

function NavLink({ item, collapsed, onClick }: { item: NavItem; collapsed: boolean; onClick?: () => void }) {
  const { pathname } = useLocation()
  const active = item.to === '/admin' ? pathname === item.to : pathname === item.to || pathname.startsWith(`${item.to}/`)
  return (
    <Link to={item.to} className={`nav-link ${active ? 'active' : ''}`} ariaLabel={collapsed ? item.label : undefined} onClick={onClick}>
      {item.icon}{!collapsed && <span>{item.label}</span>}
      {collapsed && <span className="nav-tooltip">{item.label}</span>}
    </Link>
  )
}

export function AppShell({ children, title, eyebrow, actions }: { children: ReactNode; title?: string; eyebrow?: string; actions?: ReactNode }) {
  const { user, signOut } = useAuth()
  const { productName, version } = useBrand()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('ptium.nav_collapsed') === 'true')
  const [mobileOpen, setMobileOpen] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [connection, setConnection] = useState<'checking' | 'ready' | 'error'>('checking')
  const location = useLocation()

  useEffect(() => { setMobileOpen(false); setAccountOpen(false); setSearchOpen(false) }, [location.pathname])
  useEffect(() => { localStorage.setItem('ptium.nav_collapsed', String(collapsed)) }, [collapsed])
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); setSearchOpen(true) }
      if (event.key === 'Escape') { setSearchOpen(false); setAccountOpen(false); setMobileOpen(false) }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])
  useEffect(() => {
    let active = true
    const check = () => { void api.readiness().then((ready) => { if (active) setConnection(ready ? 'ready' : 'error') }) }
    check()
    const interval = window.setInterval(check, 30000)
    return () => { active = false; window.clearInterval(interval) }
  }, [])

  const initials = useMemo(() => (user?.name || user?.email || 'P').split(/\s+/).map((part) => part[0]).slice(0, 2).join('').toUpperCase(), [user])
  const logout = async () => { await signOut(); navigate('/login', true) }
  const submitSearch = (event: React.FormEvent) => {
    event.preventDefault()
    const query = searchQuery.trim()
    setSearchOpen(false)
    navigate(query ? `/presentations?q=${encodeURIComponent(query)}` : '/presentations')
  }

  const navContent = (mobile = false) => (
    <>
      <div className="sidebar-head">
        <PtiumLogo compact={collapsed && !mobile} />
        {mobile ? <button className="icon-button" onClick={() => setMobileOpen(false)} aria-label="메뉴 닫기"><X size={19} /></button> : (
          <button className="sidebar-collapse" onClick={() => setCollapsed((value) => !value)} aria-label={collapsed ? '메뉴 펼치기' : '메뉴 접기'}>
            {collapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}
          </button>
        )}
      </div>
      <nav className="sidebar-nav" aria-label="주 메뉴">
        <div className="nav-section">
          {!collapsed || mobile ? <span className="nav-label">워크스페이스</span> : null}
          {workspaceNav.map((item) => <NavLink key={item.to} item={item} collapsed={collapsed && !mobile} onClick={mobile ? () => setMobileOpen(false) : undefined} />)}
        </div>
        <div className="nav-section">
          {!collapsed || mobile ? <span className="nav-label">내 설정</span> : null}
          {settingsNav.map((item) => <NavLink key={item.to} item={item} collapsed={collapsed && !mobile} onClick={mobile ? () => setMobileOpen(false) : undefined} />)}
        </div>
        {user?.role === 'admin' && (
          <div className="nav-section admin-nav">
            {!collapsed || mobile ? <span className="nav-label">관리</span> : null}
            {adminNav.map((item) => <NavLink key={item.to} item={item} collapsed={collapsed && !mobile} onClick={mobile ? () => setMobileOpen(false) : undefined} />)}
          </div>
        )}
      </nav>
      <div className="sidebar-foot">
        <div className={`plan-card ${collapsed && !mobile ? 'compact' : ''}`}>
          <span className="plan-icon"><Sparkles size={16} /></span>
          {(!collapsed || mobile) && <div><span>셀프 호스팅</span><strong>{connection === 'ready' ? 'API · DB 준비됨' : connection === 'error' ? '연결 확인 필요' : '상태 확인 중'}</strong><div className="mini-progress"><i style={{ width: connection === 'ready' ? '100%' : connection === 'checking' ? '40%' : '0%' }} /></div></div>}
        </div>
      </div>
    </>
  )

  return (
    <div className={`app-shell ${collapsed ? 'nav-collapsed' : ''}`}>
      <aside className="sidebar">{navContent()}</aside>
      {mobileOpen && <div className="mobile-nav-backdrop" onClick={() => setMobileOpen(false)}><aside className="mobile-sidebar" onClick={(event) => event.stopPropagation()}>{navContent(true)}</aside></div>}
      <div className="app-main">
        <header className="topbar">
          <div className="topbar-left">
            <button className="icon-button mobile-menu" onClick={() => setMobileOpen(true)} aria-label="메뉴 열기"><Menu size={20} /></button>
            <button className="search-trigger" onClick={() => setSearchOpen(true)}><Search size={17} /><span>프레젠테이션 검색</span><kbd>{/Mac|iPhone|iPad/.test(navigator.platform) ? '⌘ K' : 'Ctrl K'}</kbd></button>
          </div>
          <div className="topbar-actions">
            <Link to="/create" className="quick-create"><Plus size={17} /><span>새 프레젠테이션</span></Link>
            <div className="account-wrap">
              <button className="account-trigger" onClick={() => setAccountOpen((value) => !value)} aria-expanded={accountOpen}>
                <span className="avatar">{initials}</span><span className="account-copy"><strong>{user?.name || '사용자'}</strong><small>{user?.role === 'admin' ? '관리자' : '멤버'}</small></span><ChevronDown size={15} />
              </button>
              {accountOpen && <div className="account-menu">
                <div className="account-menu-head"><strong>{user?.name}</strong><span>{user?.email}</span></div>
                <Link to="/profile"><CircleUserRound size={16} /> 내 프로필</Link>
                <Link to="/guide"><LifeBuoy size={16} /> 사용 가이드</Link>
                <button onClick={() => void logout()}><LogOut size={16} /> 로그아웃</button>
                {/* The build, so a report and a release can be matched up. */}
                <div className="account-menu-foot">
                  <span><Info size={13} /> {productName} {versionLabel(version)}</span>
                  <button
                    type="button"
                    className="account-menu-copy"
                    onClick={() => { void navigator.clipboard?.writeText(`${productName} ${versionLabel(version)}`); setAccountOpen(false) }}
                  >복사</button>
                </div>
              </div>}
            </div>
          </div>
        </header>
        <main className="page-wrap">
          {(title || actions) && <div className="page-heading"><div>{eyebrow && <span className="eyebrow">{eyebrow}</span>}{title && <h1>{title}</h1>}</div>{actions && <div className="page-actions">{actions}</div>}</div>}
          {children}
        </main>
      </div>
      {searchOpen && <div className="command-backdrop" onClick={() => setSearchOpen(false)}>
        <form className="command-palette" onClick={(event) => event.stopPropagation()} onSubmit={submitSearch}>
          <div className="command-input"><Search size={19} /><input autoFocus value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} placeholder="프레젠테이션 제목을 검색하세요" aria-label="검색" /><button type="button" onClick={() => setSearchOpen(false)}>ESC</button></div>
          <div className="command-empty"><Search size={24} /><strong>제목 검색</strong><span>검색어를 입력하고 Enter를 누르세요.</span></div>
        </form>
      </div>}
    </div>
  )
}
