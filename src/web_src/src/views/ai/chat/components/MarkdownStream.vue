<script setup>
// Shared streaming-Markdown renderer for chat (assistant answers + reasoning).
// Uses streamdown-vue (the Vue port of Vercel's Streamdown): GitHub-flavored
// markdown (tables, task lists, strikethrough), KaTeX math, Shiki code blocks
// with copy/download buttons, and — via parseIncompleteMarkdown — graceful
// rendering of half-streamed tokens (unterminated ``` / **bold** / tables)
// while the answer is still arriving.
//
// streamdown-vue renders to plain growing DOM (no internal virtual scroller),
// so the chat page's own auto-scroll can follow the output.
import { StreamMarkdown } from 'streamdown-vue'
import remarkMath from 'remark-math'

defineProps({
  content: { type: String, default: '' },
  // Kept for a uniform call-site API; streamdown repairs incomplete markdown
  // continuously, so no separate "final" flag is needed.
  streaming: { type: Boolean, default: false },
})

// Fix mangled KaTeX class names. rehype-katex emits hast where
// `properties.className` is an ARRAY (e.g. ["mord", "mathnormal"]). streamdown
// spreads hast properties straight into Vue's h(); Vue only normalizes the
// `class` prop, not `className`, so it assigns the array to el.className, which
// stringifies with JS's default COMMA join → class="mord,mathnormal". A comma
// is a legal char inside one class name, so the browser sees a single bogus
// class and EVERY KaTeX selector (.mathnormal, .mord, …) misses — math falls
// back to the inherited UI font. Flatten each className array to a space-joined
// string, after rehype-katex has run, so Vue emits a correct class list.
//
// Scope this to KaTeX subtrees ONLY. streamdown's own code path reads a code
// block's `className` as an array (`(className || []).find(c => c.startsWith
// ("language-"))`); flattening it to a string makes `.find` throw and crashes
// the whole render. So we flatten only once we're inside a `katex*` element.
function rehypeJoinClassNames() {
  const isKatexRoot = (cn) =>
    Array.isArray(cn) && cn.some((c) => typeof c === 'string' && c.startsWith('katex'))
  return (tree) => {
    const walk = (node, inMath) => {
      if (!node || typeof node !== 'object') return
      const cn = node.properties && node.properties.className
      const here = inMath || isKatexRoot(cn)
      if (here && Array.isArray(cn)) node.properties.className = cn.join(' ')
      if (node.children) node.children.forEach((child) => walk(child, here))
    }
    walk(tree, false)
  }
}
// Stable reference so the prop doesn't change identity on every render.
const rehypePlugins = [rehypeJoinClassNames]

// Enable single-dollar inline math ($c^2$). streamdown registers remark-math
// itself with `singleDollarTextMath: false` (only $$…$$ renders), UNLESS we
// supply our own remark-math instance — it dedupes by reference and skips its
// default. So we pass the SAME remark-math (deduped to streamdown's copy via
// our matching ^6.0.0 dep) with single-dollar turned back on. Without this,
// inline `$…$` leaks through as literal text.
const remarkPlugins = [[remarkMath, { singleDollarTextMath: true }]]

// Code-block syntax theme: a light/dark pair so Shiki's token colors follow the
// app's color mode instead of being pinned dark. streamdown emits per-token CSS
// vars (--shiki-light / --shiki-dark) and flips them via `html.dark` — the same
// class VueUse's useColorMode sets on <html> — so the highlighting (and the
// matching chrome in chat-markdown.css) switches automatically with the UI.
// Stable reference so the prop keeps its identity across streaming re-renders.
const shikiTheme = { light: 'github-light', dark: 'github-dark' }
</script>

<template>
  <StreamMarkdown
    class="chat-md"
    :content="content || ''"
    :remark-plugins="remarkPlugins"
    :rehype-plugins="rehypePlugins"
    :parse-incomplete-markdown="true"
    :shiki-theme="shikiTheme"
    :allowed-link-prefixes="['*']"
    :allowed-image-prefixes="['*']"
  />
</template>
