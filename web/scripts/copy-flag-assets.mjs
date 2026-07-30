import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'

const source = resolve('node_modules/flag-icons/flags/4x3')
const destination = resolve('public/assets/flags')

if (!existsSync(source)) {
  throw new Error('flag-icons 4x3 assets are missing; run npm ci first')
}

rmSync(destination, { recursive: true, force: true })
mkdirSync(destination, { recursive: true })
cpSync(source, destination, { recursive: true })
