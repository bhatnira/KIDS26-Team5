import { createApp } from 'vue'
import App from './App.vue'

// highlight.js stylesheet for syntax-highlighted code blocks in the chat
// (and anywhere else that renders .hljs spans). One dark theme is enough
// because chat code blocks are always rendered on a dark background, even
// in light mode — the claude.ai pattern.
import 'highlight.js/styles/atom-one-dark.css'

// Streaming-Markdown renderer (chat): streamdown-vue base styles + KaTeX CSS
// for the math it renders via rehype-katex.
import 'streamdown-vue/style.css'
import 'katex/dist/katex.min.css'
// Our overrides — imported AFTER streamdown's base styles so they win on ties.
import '@/styles/chat-markdown.css'

const app = createApp(App)

app.mount('#app')
