import { safeRedirect } from '../urlValidation'

describe('urlValidation', () => {
  describe('safeRedirect', () => {
    it('throws error for disallowed domain', () => {
      expect(() => {
        safeRedirect('https://evil.com/phishing', ['example.com'])
      }).toThrow('Redirect blocked')
    })

    it('throws error for invalid URL format', () => {
      expect(() => {
        safeRedirect('not-a-url', ['example.com'])
      }).toThrow()
    })

    it('throws error for URL that looks like allowed domain but is not', () => {
      expect(() => {
        safeRedirect('https://example.com.evil.com/test', ['example.com'])
      }).toThrow('Redirect blocked')
    })

    it('throws error for URLs with credentials (phishing protection)', () => {
      expect(() => {
        safeRedirect('https://user:pass@evil.com', ['evil.com'])
      }).toThrow('URLs with embedded credentials are not allowed')

      expect(() => {
        safeRedirect('https://example.com@evil.com/phishing', ['example.com'])
      }).toThrow('URLs with embedded credentials are not allowed')
    })

    it('rejects a non-github.com install URL (GitHub App flow)', () => {
      expect(() => {
        safeRedirect('https://github.com.evil.com/installations/new', [
          'github.com',
        ])
      }).toThrow('Redirect blocked')
    })

    it('rejects non-https schemes even when the host is allowlisted', () => {
      expect(() => {
        safeRedirect('http://example.com/test', ['example.com'])
      }).toThrow('only https URLs are allowed')
    })

    it('rejects javascript: and data: schemes', () => {
      expect(() => {
        safeRedirect('javascript:alert(1)', ['example.com'])
      }).toThrow()
      expect(() => {
        safeRedirect('data:text/html,<script>alert(1)</script>', [
          'example.com',
        ])
      }).toThrow()
    })

    it('validates URL before redirect', () => {
      // Should not throw for valid domain
      const validUrl = 'https://github.com/test'
      expect(() => {
        try {
          safeRedirect(validUrl, ['github.com'])
        } catch (e) {
          // Ignore navigation errors in test environment
          if (e instanceof Error && !e.message.includes('Redirect blocked')) {
            return
          }
          throw e
        }
      }).not.toThrow('Redirect blocked')
    })
  })
})
