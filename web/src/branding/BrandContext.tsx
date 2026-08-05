import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api } from '../api/client'

interface BrandState {
  productName: string
  logoUrl: string
  brandColor: string
  /** The build the server is running, as it reports itself. Empty until it answers. */
  version: string
}

const fallbackBrand: BrandState = { productName: 'Ptium', logoUrl: '', brandColor: '#725BD6', version: '' }
const BrandContext = createContext<BrandState>(fallbackBrand)

export function BrandProvider({ children }: { children: ReactNode }) {
  const [brand, setBrand] = useState(fallbackBrand)

  useEffect(() => {
    let active = true
    const load = () => api.publicSettings().then((settings) => {
        if (!active) return
        const candidate = String(settings['branding.brand_color'] || fallbackBrand.brandColor)
        const brandColor = /^#[0-9a-f]{6}$/i.test(candidate) ? candidate : fallbackBrand.brandColor
        setBrand({
          productName: String(settings['branding.product_name'] || fallbackBrand.productName).trim() || fallbackBrand.productName,
          logoUrl: String(settings['branding.logo_url'] || '').trim(),
          brandColor,
          version: String(settings['service.version'] || '').trim(),
        })
      }).catch(() => { /* The built-in brand remains available while the API starts. */ })
    void load()
    window.addEventListener('ptium:branding-updated', load)
    return () => { active = false; window.removeEventListener('ptium:branding-updated', load) }
  }, [])

  useEffect(() => {
    document.documentElement.style.setProperty('--violet', brand.brandColor)
    document.documentElement.style.setProperty('--violet-dark', shade(brand.brandColor, -24))
    document.documentElement.style.setProperty('--violet-soft', `${brand.brandColor}1a`)
    document.title = `${brand.productName} · Presentation Studio`
  }, [brand])

  const value = useMemo(() => brand, [brand])
  return <BrandContext.Provider value={value}>{children}</BrandContext.Provider>
}

export function useBrand() {
  return useContext(BrandContext)
}

export function BrandMark({ size = 'default' }: { size?: 'default' | 'large' | 'tiny' }) {
  const { logoUrl, productName } = useBrand()
  const suffix = size === 'default' ? '' : ` ${size}`
  if (logoUrl) return <img className={`brand-logo-image${suffix}`} src={logoUrl} alt={`${productName} 로고`} />
  return <span className={`brand-mark${suffix}`} aria-hidden="true"><i /><i /><i /></span>
}

function shade(hex: string, amount: number) {
  const value = Number.parseInt(hex.slice(1), 16)
  const channel = (shift: number) => Math.max(0, Math.min(255, (value >> shift & 255) + amount)).toString(16).padStart(2, '0')
  return `#${channel(16)}${channel(8)}${channel(0)}`
}
