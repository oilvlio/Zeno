import path from 'node:path'

function htmlAttribute(tag, name) {
  const match = tag.match(new RegExp(`\\b${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s>]+))`, 'i'))
  return match ? (match[1] ?? match[2] ?? match[3] ?? '') : null
}

export function moduleEntryAssetURLs(indexHTML) {
  const entries = []
  const tags = indexHTML.match(/<(?:script|link)\b[^>]*>/gi) ?? []
  for (const tag of tags) {
    if (!/^<script\b/i.test(tag) || htmlAttribute(tag, 'type')?.toLowerCase() !== 'module') continue
    const src = htmlAttribute(tag, 'src')
    if (src) entries.push(src)
  }
  return [...new Set(entries)]
}

export function staticModuleAssetURLs(indexHTML) {
  const urls = [...moduleEntryAssetURLs(indexHTML)]
  const tags = indexHTML.match(/<link\b[^>]*>/gi) ?? []
  for (const tag of tags) {
    const rel = htmlAttribute(tag, 'rel')
      ?.toLowerCase()
      .split(/\s+/)
      .filter(Boolean) ?? []
    if (!rel.includes('modulepreload')) continue
    const href = htmlAttribute(tag, 'href')
    if (href) urls.push(href)
  }
  return [...new Set(urls)]
}

export function resolveDistAssetPath(distDir, assetURL) {
  const baseURL = new URL('https://zeno.invalid/')
  const parsed = new URL(assetURL, baseURL)
  if (parsed.origin !== baseURL.origin) {
    throw new Error(`external initial JavaScript asset is not supported: ${assetURL}`)
  }
  const relativeAssetPath = decodeURIComponent(parsed.pathname).replace(/^\/+/, '')
  const absoluteDistDir = path.resolve(distDir)
  const resolved = path.resolve(absoluteDistDir, relativeAssetPath)
  const relative = path.relative(absoluteDistDir, resolved)
  if (relative === '' || relative === '..' || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new Error(`initial JavaScript asset escapes dist: ${assetURL}`)
  }
  return resolved
}
