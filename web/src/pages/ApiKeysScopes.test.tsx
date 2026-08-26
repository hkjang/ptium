import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ScopeChooser, revoked } from './ApiKeysPage'

// The list of permissions belongs to the server. This page used to keep its
// own, and it had drifted: templates:read — which seven routes require and
// every default key carries — was not on it, so nobody could grant it.
describe('the permission list', () => {
  const catalogue = [
    { id: 'presentations:read', grants: 'read decks' },
    { id: 'templates:read', grants: 'read templates' },
    { id: 'admin:users', admin: true, grants: 'read the account list' },
    { id: 'weather:read', grants: 'a scope this file has never heard of' },
  ]

  it('offers whatever the server offers, in the product\'s words where it has them', () => {
    render(<ScopeChooser catalogue={catalogue} chosen={['templates:read']} onToggle={() => {}} />)
    expect(screen.getByText('템플릿 조회 · 미리보기')).toBeTruthy()
    expect(screen.getByText('프레젠테이션 조회 · 내보내기')).toBeTruthy()
    // A scope this file has no words for is still offered, named as the server
    // names it: the screen shows what the server has, not what this file knows.
    expect(screen.getAllByText('weather:read').length).toBeGreaterThan(0)
    expect(screen.getByText('관리자')).toBeTruthy()
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(boxes).toHaveLength(4)
    expect(boxes.filter((box) => box.checked)).toHaveLength(1)
  })

  it('says so rather than showing an empty list', () => {
    render(<ScopeChooser catalogue={[]} chosen={[]} onToggle={() => {}} />)
    expect(screen.getByText('권한 목록을 불러오지 못했습니다.')).toBeTruthy()
  })
})

// A revoked key is over. A key rotated away still works while its grace lasts.
describe('which keys belong on the list', () => {
  it('leaves out the revoked and keeps the rotating', () => {
    expect(revoked({ status: 'revoked' } as never)).toBe(true)
    expect(revoked({ status: 'rotating' } as never)).toBe(false)
    expect(revoked({ status: 'active' } as never)).toBe(false)
    expect(revoked({ status: 'expired' } as never)).toBe(false)
  })
})
