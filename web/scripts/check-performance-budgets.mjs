import { gzipSync } from 'node:zlib'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'

const distDir = path.resolve('dist')
const indexHTML = await readFile(path.join(distDir, 'index.html'), 'utf8')
const entryMatch = indexHTML.match(/<script[^>]+type="module"[^>]+src="([^"]+)"/)
if (!entryMatch) throw new Error('cannot resolve the production entry from dist/index.html')

const entryPath = path.join(distDir, entryMatch[1].replace(/^\//, ''))
const entry = await readFile(entryPath)
const entryRawBytes = entry.byteLength
const entryGzipBytes = gzipSync(entry, { level: 9 }).byteLength
const maxEntryRawBytes = 260 * 1024
const maxEntryGzipBytes = 82 * 1024
if (entryRawBytes > maxEntryRawBytes || entryGzipBytes > maxEntryGzipBytes) {
  throw new Error(`public entry exceeds budget: raw=${entryRawBytes}/${maxEntryRawBytes}, gzip=${entryGzipBytes}/${maxEntryGzipBytes}`)
}

const assetsDir = path.join(distDir, 'assets')
const assets = await readdir(assetsDir)
const requireChunk = (prefix) => {
  const matches = assets.filter((name) => name.startsWith(`${prefix}-`) && name.endsWith('.js'))
  if (matches.length !== 1) throw new Error(`expected one ${prefix} chunk, found ${matches.length}`)
  return matches[0]
}

const chunks = {
  admin: requireChunk('AdminDashboard'),
  nodeDetail: requireChunk('NodeDetailRoute'),
  serviceDetail: requireChunk('ServiceDetailRoute'),
}
for (const [sentinel, owner] of [
  ['账号只能使用 3-64 位', 'account'],
  ['主题与背景', 'settings'],
]) {
  if (entry.includes(sentinel)) {
    throw new Error(`${owner} module leaked into the public entry`)
  }
}
for (const sentinel of ['同步监控服务…', '暂无服务延迟历史']) {
  if (entry.includes(sentinel)) throw new Error(`detail module leaked into the public entry: ${sentinel}`)
}

const flagDir = path.join(distDir, 'assets', 'flags')
const flags = (await readdir(flagDir)).filter((name) => name.endsWith('.svg'))
if (flags.length === 0 || flags.length > 300) {
  throw new Error(`unexpected 4:3 flag asset count: ${flags.length}`)
}
const distFiles = await readdir(assetsDir)
let chunkBytes = 0
for (const name of distFiles.filter((value) => value.endsWith('.js'))) {
  chunkBytes += (await stat(path.join(assetsDir, name))).size
}
const maxJavaScriptBytes = 420 * 1024
if (chunkBytes > maxJavaScriptBytes) {
  throw new Error(`total production JavaScript exceeds budget: ${chunkBytes}/${maxJavaScriptBytes}`)
}

console.log(JSON.stringify({
  entry: path.basename(entryPath),
  entryRawBytes,
  entryGzipBytes,
  chunks,
  javascriptBytes: chunkBytes,
  flagAssets: flags.length,
}, null, 2))
