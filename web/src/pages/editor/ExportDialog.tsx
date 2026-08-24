import { Download, FileText } from 'lucide-react'
import { Button, LoadingState, Modal } from '../../components/UI'

/**
 * Taking the deck out of Ptium.
 *
 * Both formats are the same deck and they are not the same file. The pptx is
 * the deck in its own template — its typeface, its master, editable. The PDF is
 * the deck as a page: it opens anywhere, it cannot be edited, and it is set in
 * the one face the workspace carries, because a template names its typeface and
 * does not carry it. The dialog says so rather than letting someone find out
 * after they have sent it.
 */
export function ExportDialog({ open, exporting, onExport, onClose }: {
  open: boolean
  exporting: boolean
  onExport: (format: 'pptx' | 'pdf') => void
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
        <button disabled={exporting} onClick={() => onExport('pdf')}>
          <span className="export-icon pdf"><FileText size={22} /></span>
          <div>
            <strong>PDF 문서 (.pdf)</strong>
            <p>어디서나 열리는 배포용. 링크와 발표 노트의 주소까지 살아 있고, 글꼴은 워크스페이스가 싣고 있는 나눔바른고딕입니다.</p>
          </div>
          <Download size={18} />
        </button>
      </div>
      {exporting && <LoadingState compact label="파일을 준비하고 있어요…" />}
    </Modal>
  )
}
