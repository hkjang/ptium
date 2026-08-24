import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ExportDialog } from './ExportDialog'

describe('taking the deck out', () => {
  it('offers both files, and each asks for itself', () => {
    const onExport = vi.fn()
    render(<ExportDialog open exporting={false} onExport={onExport} onClose={() => {}} />)
    fireEvent.click(screen.getByText('PowerPoint (.pptx)'))
    expect(onExport).toHaveBeenCalledWith('pptx')
    fireEvent.click(screen.getByText('PDF 문서 (.pdf)'))
    expect(onExport).toHaveBeenCalledWith('pdf')
  })

  // The two files are not the same deck in two wrappers, and somebody who sends
  // the PDF should know that before they send it rather than after.
  it('says what the PDF is set in', () => {
    render(<ExportDialog open exporting={false} onExport={() => {}} onClose={() => {}} />)
    expect(screen.getByText(/나눔바른고딕/)).toBeTruthy()
  })

  it('cannot be asked twice while a file is being made', () => {
    render(<ExportDialog open exporting onExport={() => {}} onClose={() => {}} />)
    for (const label of ['PowerPoint (.pptx)', 'PDF 문서 (.pdf)']) {
      expect(screen.getByText(label).closest('button')?.hasAttribute('disabled'), label).toBe(true)
    }
    expect(screen.getByText(/파일을 준비하고 있어요/)).toBeTruthy()
  })
})
