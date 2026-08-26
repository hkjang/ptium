import { ArrowLeft, Home, LoaderCircle, ShieldAlert } from 'lucide-react'
import { useAuth } from './auth/AuthContext'
import { BrandMark, useBrand } from './branding/BrandContext'
import { Button } from './components/UI'
import { AdminAuditPage } from './pages/AdminAuditPage'
import { AdminQueuePage } from './pages/AdminQueuePage'
import { AdminSharesPage } from './pages/AdminSharesPage'
import { AdminUsagePage } from './pages/AdminUsagePage'
import { AdminTemplatesPage } from './pages/AdminTemplatesPage'
import { AdminErrorsPage } from './pages/AdminErrorsPage'
import { AdminOverviewPage } from './pages/AdminOverviewPage'
import { AdminSettingsPage } from './pages/AdminSettingsPage'
import { AdminUsersPage } from './pages/AdminUsersPage'
import { ApiKeysPage } from './pages/ApiKeysPage'
import { ApiDocsPage } from './pages/ApiDocsPage'
import { AssetsPage } from './pages/AssetsPage'
import { CreatePage } from './pages/CreatePage'
import { DashboardPage } from './pages/DashboardPage'
import { EditorPage } from './pages/EditorPage'
import { GuidePage } from './pages/GuidePage'
import { LoginPage } from './pages/LoginPage'
import { PresentationsPage } from './pages/PresentationsPage'
import { PresenterPage } from './pages/PresenterPage'
import { SharedDeckPage } from './pages/SharedDeckPage'
import { ProfilePage } from './pages/ProfilePage'
import { TemplatesPage } from './pages/TemplatesPage'
import { Link, navigate, useLocation } from './router'

export function App() {
  const { pathname } = useLocation()
  const { user, loading } = useAuth()
  const { productName } = useBrand()

  // A shared deck opens before anything else asks who is looking: the link is
  // for someone who has no account here, and sending them to a sign-in page
  // would defeat it.
  const sharedMatch = pathname.match(/^\/view\/([^/]+)$/)
  if (sharedMatch) return <SharedDeckPage token={decodeURIComponent(sharedMatch[1])} />

  if (loading || pathname === '/auth/callback') return <main className="app-bootstrap"><div className="login-brand"><BrandMark size="large" /><span>{productName}</span></div><LoaderCircle className="spin" size={24} /><p>워크스페이스를 준비하는 중…</p></main>
  if (pathname === '/login') return <LoginPage />
  if (!user) { navigate('/login', true); return null }

  if (pathname === '/' || pathname === '/dashboard') return <DashboardPage />
  if (pathname === '/presentations') return <PresentationsPage />
  if (pathname === '/templates') return <TemplatesPage />
  if (pathname === '/images') return <AssetsPage />
  if (pathname === '/create') return <CreatePage />
  const editorMatch = pathname.match(/^\/presentations\/([^/]+)\/editor$/)
  if (editorMatch) return <EditorPage id={decodeURIComponent(editorMatch[1])} />
  // The presenter's second window. It carries no workspace chrome: it is the
  // screen the speaker looks at while the projector shows the deck.
  const presenterMatch = pathname.match(/^\/presentations\/([^/]+)\/presenter$/)
  if (presenterMatch) return <PresenterPage id={decodeURIComponent(presenterMatch[1])} />
  if (pathname === '/profile') return <ProfilePage />
  if (pathname === '/api-keys') return <ApiKeysPage />
  if (pathname === '/guide') return <GuidePage />
  if (pathname === '/docs') return <ApiDocsPage />

  if (pathname.startsWith('/admin') && user.role !== 'admin') return <AccessDenied />
  if (pathname === '/admin') return <AdminOverviewPage />
  if (pathname === '/admin/settings') return <AdminSettingsPage />
  if (pathname === '/admin/users') return <AdminUsersPage />
  if (pathname === '/admin/errors') return <AdminErrorsPage />
  if (pathname === '/admin/audit') return <AdminAuditPage />
  if (pathname === '/admin/queue') return <AdminQueuePage />
  if (pathname === '/admin/shares') return <AdminSharesPage />
  if (pathname === '/admin/usage') return <AdminUsagePage />
  if (pathname === '/admin/designs') return <AdminTemplatesPage />
  return <NotFound />
}

function AccessDenied() {
  return <main className="standalone-state"><span className="standalone-icon"><ShieldAlert size={28} /></span><span className="eyebrow">403 · ACCESS DENIED</span><h1>관리자 권한이 필요합니다</h1><p>이 영역은 서비스 관리자만 사용할 수 있어요.</p><Button onClick={() => navigate('/dashboard')}><Home size={16} /> 워크스페이스로 돌아가기</Button></main>
}

function NotFound() {
  return <main className="standalone-state"><span className="notfound-number">404</span><span className="eyebrow">PAGE NOT FOUND</span><h1>페이지를 찾을 수 없어요</h1><p>주소가 변경되었거나 더 이상 존재하지 않는 페이지입니다.</p><Link to="/dashboard" className="button button-primary button-medium"><ArrowLeft size={16} /> 홈으로 돌아가기</Link></main>
}
