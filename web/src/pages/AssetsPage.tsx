import { useEffect, useState } from 'react'
import { Images } from 'lucide-react'
import { AppShell } from '../components/AppShell'
import { AssetLibrary } from '../components/AssetLibrary'
import { useToast } from '../components/Toast'
import { api } from '../api/client'
import { Link } from '../router'

/**
 * The image library on its own page.
 *
 * The same library lives in the editor's side panel, where it is a tool for the
 * deck being worked on. Here it is the thing itself: what someone has collected,
 * what they use, what they should probably delete. Tidying a library is not
 * something anyone does in a 320-pixel panel.
 */
export function AssetsPage() {
  const { showToast } = useToast()
  const [summary, setSummary] = useState<{ count: number; bytes: number } | null>(null)

  useEffect(() => {
    let active = true
    void api.assets({ limit: 500 }).then((items) => {
      if (!active) return
      setSummary({ count: items.length, bytes: items.reduce((total, item) => total + item.sizeBytes, 0) })
    }).catch(() => { /* the library below reports its own failures */ })
    return () => { active = false }
  }, [])

  return (
    <AppShell
      title="내 이미지"
      eyebrow="IMAGE LIBRARY"
      actions={summary ? <span className="page-note">
        <Images size={14} /> {summary.count.toLocaleString()}장 · {(summary.bytes / (1024 * 1024)).toFixed(1)}MB
      </span> : undefined}
    >
      <p className="page-intro">
        올린 이미지는 계정에 남아 모든 덱에서 다시 씁니다. 자주 쓰는 것은 별표를 눌러 맨 앞에
        두고, 로고·제품컷처럼 쓰임이 같은 것은 태그로 묶으세요. 슬라이드에 넣는 방법은{' '}
        <Link to="/guide#images">사용 가이드</Link>에 있습니다.
      </p>
      <AssetLibrary compact={false} notify={showToast} />
    </AppShell>
  )
}
