// Prism configuration - import order is critical!
// The core library MUST be imported before any language components
import Prism from 'prismjs'

// No Prism theme is imported on purpose: the code-block surface and the token
// colours are design tokens in styles/index.css (`.code-block`, `.token`), so
// code follows the app's light and dark themes instead of one fixed palette.
// Language components - these extend Prism.languages
import 'prismjs/components/prism-javascript'
import 'prismjs/components/prism-typescript'
import 'prismjs/components/prism-jsx'
import 'prismjs/components/prism-tsx'
import 'prismjs/components/prism-python'
import 'prismjs/components/prism-bash'
import 'prismjs/components/prism-json'
import 'prismjs/components/prism-yaml'
import 'prismjs/components/prism-sql'
import 'prismjs/components/prism-go'
import 'prismjs/components/prism-java'
import 'prismjs/components/prism-css'
import 'prismjs/components/prism-scss'
import 'prismjs/components/prism-markdown'

// Export configured Prism instance
export default Prism
