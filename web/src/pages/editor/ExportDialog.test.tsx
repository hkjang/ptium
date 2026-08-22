import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ExportDialog } from './ExportDialog'

describe('taking the deck out', () => {
  it('exports the format it can, and says what to do about the one it cannot', () => {
    const onExport = vi.fn()
    render(<ExportDialog open exporting={false} onExport={onExport} onClose={() => {}} />)
    fireEvent.click(screen.getByText('PowerPoint (.pptx)'))
    expect(onExport).toHaveBeenCalledWith('pptx')
    // The PDF row is present and disabled: hiding it would send people looking
    // through the menus for something that is not there.
    expect(screen.getByText(/PowerPoint나 LibreOffice에서 PDF로 저장하면/)).toBeTruthy()
  })

  it('cannot be asked twice while a file is being made', () => {
    render(<ExportDialog open exporting onExport={() => {}} onClose={() => {}} />)
    const pptx = screen.getByText('PowerPoint (.pptx)').closest('button')
    expect(pptx?.hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/파일을 준비하고 있어요/)).toBeTruthy()
  })
})
