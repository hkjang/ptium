import { Download, FileText } from 'lucide-react'
import { Button, LoadingState, Modal } from '../../components/UI'

/**
 * Taking the deck out of Ptium.
 *
 * The PDF option is present and disabled on purpose. Ptium could rasterise its
 * own preview and call it a PDF, but the fonts would be whatever the server
 * has rather than what the template asked for — so the dialog says what to do
 * instead, which is more useful than hiding the row and letting someone search
 * the menus for it.
 */
export function ExportDialog({ open, exporting, onExport, onClose }: {
  open: boolean
  exporting: boolean
  onExport: (format: 'pptx') => void
  onClose: () => void
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="프레젠테이션 내보내기"
      description="사용할 형식을 선택하세요."
      footer={<Button variant="secondary" onClick={onClose}>취소</Button>}
    >
      <div className="export-options">
        <button disabled={exporting} onClick={() => onExport('pptx')}>
          <span className="export-icon ppt"><FileText size={22} /></span>
          <div>
            <strong>PowerPoint (.pptx)</strong>
            <p>Microsoft PowerPoint와 호환되는 편집 가능한 파일</p>
          </div>
          <Download size={18} />
        </button>
        <button disabled title="받은 PPTX를 PowerPoint·LibreOffice에서 PDF로 저장하세요">
          <span className="export-icon pdf"><FileText size={22} /></span>
          <div>
            <strong>PDF 문서 (.pdf)</strong>
            <p>아직 제공하지 않습니다. 받은 PPTX를 PowerPoint나 LibreOffice에서 PDF로 저장하면 글꼴이 정확합니다.</p>
          </div>
        </button>
      </div>
      {exporting && <LoadingState compact label="파일을 준비하고 있어요…" />}
    </Modal>
  )
}
