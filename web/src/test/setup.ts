import { afterEach } from 'vitest'

// A rendered dialog has to leave the document when its test ends, or the next
// test finds two of every button. The rules-only tests run without a document
// and skip this.
if (typeof document !== 'undefined') {
  const { cleanup } = await import('@testing-library/react')
  afterEach(cleanup)
}
