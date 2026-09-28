import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8000'
const output = path.join(root, '.data', 'screenshots')
fs.mkdirSync(output, { recursive: true })
const browser = await puppeteer.launch({
  executablePath: process.env.CHROME_PATH || '/usr/bin/chromium',
  headless: true,
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
})
const page = await browser.newPage()
page.setDefaultTimeout(18000)
await page.setViewport({ width: 1536, height: 1024, deviceScaleFactor: 1 })
const errors = []
const checks = []
page.on('pageerror', (e) => errors.push(e.message))
async function clickText(text, scope = 'button') {
  await page.waitForFunction(
    (text, scope) =>
      [...document.querySelectorAll(scope)].some(
        (el) => el.textContent.includes(text) && el.offsetParent !== null,
      ),
    {},
    text,
    scope,
  )
  await page.evaluate(
    (text, scope) => {
      const el = [...document.querySelectorAll(scope)].find(
        (el) => el.textContent.includes(text) && el.offsetParent !== null,
      )
      el.click()
    },
    text,
    scope,
  )
}
async function request(route) {
  return page.evaluate(async (route) => {
    const response = await fetch('/api' + route)
    if (!response.ok) throw new Error('API failure: ' + response.status)
    return response.json()
  }, route)
}
async function shot(name) {
  const dismiss = await page.$('.toast button')
  if (dismiss) await dismiss.click()
  await page.screenshot({ path: path.join(output, name + '.png'), fullPage: false })
}
try {
  await page.goto(base, { waitUntil: 'networkidle0' })
  await shot('login-desktop')
  await clickText('第一次来')
  const email = 'e2e-' + Date.now() + '@linyu.test'
  await page.type('input[name=name]', '端到端测试居民')
  await page.type('input[name=email]', email)
  await page.type('input[name=password]', 'browser-test-password-123')
  await page.click('form .button.primary')
  await page.waitForSelector('.conversation')
  assert.equal((await request('/agents')).length, 2)
  checks.push('registration and persistent starter agents')

  await page.click('button[aria-label=发现]')
  await clickText('我的宇宙', '.segmented button')
  const eventResponse = page.waitForResponse(
    (r) => r.url().endsWith('/advance') && r.request().method() === 'POST',
  )
  await clickText('开启一次活动')
  assert.equal((await eventResponse).status(), 200)
  await page.waitForSelector('.timeline-event')
  await shot('universe-desktop')
  checks.push('real world activity and timeline')

  await page.click('button[aria-label=聊天]')
  await clickText('记住一件事', '.composer-tools button')
  const memo = '浏览器回归：我喜欢周六上午骑自行车'
  await page.type('textarea[name=content]', memo)
  await page.click('.modal-form .button.primary')
  await page.waitForFunction(
    (memo) =>
      [...document.querySelectorAll('.memory-snippet')].some((el) => el.textContent.includes(memo)),
    {},
    memo,
  )
  await page.click('button[aria-label=我的]')
  await page.waitForSelector('.memory-row')
  await page.evaluate(
    (memo) =>
      [...document.querySelectorAll('.memory-row')]
        .find((el) => el.textContent.includes(memo))
        .querySelector('button')
        .click(),
    memo,
  )
  await page.waitForFunction(
    (memo) =>
      ![...document.querySelectorAll('.memory-row')].some((el) => el.textContent.includes(memo)),
    {},
    memo,
  )
  checks.push('memory creation and forgetting through UI')

  await page.click('button[aria-label=聊天]')
  await page.click('button[aria-label=好友资料]')
  const migrationResponse = page.waitForResponse((r) => r.url().endsWith('/migrate'))
  await page.select('.profile-modal label:last-of-type select', 'platform-universe')
  assert.equal((await migrationResponse).status(), 200)
  await page.waitForFunction(
    () =>
      document.querySelector('.profile-modal label:last-of-type select')?.value ===
      'platform-universe',
  )
  await page.click('.modal-top button[aria-label=关闭]')
  assert.equal((await request('/agents'))[0].universe_id, 'platform-universe')
  checks.push('universe migration and preserved identity')

  await clickText('委托任务', '.composer-tools button')
  await page.type('textarea[name=goal]', '写一份关于周末计划的 Markdown 文档')
  await page.click('.modal-form .button.primary')
  await page.waitForSelector('.task-detail .status.waiting_configuration')
  assert.equal((await request('/tasks'))[0].state, 'waiting_configuration')
  await page.click('.modal-top button[aria-label=关闭]')
  checks.push('task honestly waits for missing model configuration')
  await shot('chat-desktop')

  await page.click('button[aria-label=好友]')
  await shot('friends-desktop')
  await page.click('button[aria-label=发现]')
  await clickText('朋友圈', '.segmented button')
  await page.waitForSelector('.post-card')
  await shot('moments-desktop')

  await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 1 })
  await page.click('button[aria-label=聊天]')
  await page.click('.conversation')
  await page.waitForSelector('.mobile-open .chat-panel')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
  await shot('chat-mobile')
  await page.click('button[aria-label=返回聊天列表]')
  await page.click('button[aria-label=发现]')
  await clickText('我的宇宙', '.segmented button')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
  await shot('universe-mobile')
  checks.push('mobile navigation and no horizontal overflow')

  if (process.env.E2E_ADMIN_EMAIL && process.env.E2E_ADMIN_PASSWORD) {
    await page.click('button[aria-label=我的]')
    await clickText('退出登录')
    await page.waitForSelector('input[name=email]')
    await page.type('input[name=email]', process.env.E2E_ADMIN_EMAIL)
    await page.type('input[name=password]', process.env.E2E_ADMIN_PASSWORD)
    await page.click('form .button.primary')
    await page.waitForSelector('.conversation')
    await page.setViewport({ width: 1536, height: 1024, deviceScaleFactor: 1 })
    await page.goto(base + '/admin', { waitUntil: 'networkidle0' })
    await page.waitForSelector('tbody tr')
    const states = await request('/admin/users')
    assert(states.some((u) => u.email === email))
    assert.equal(states.find((u) => u.email === email).presence, 'offline')
    await shot('admin-desktop')
    await page.type('input[aria-label=搜索用户]', email)
    await page.waitForFunction(() => document.querySelectorAll('tbody tr').length === 1)
    await clickText('查看', 'tbody button')
    await page.waitForSelector('.admin-detail')
    assert((await page.$eval('.admin-detail', (el) => el.textContent)).includes('平台宇宙'))
    await page.click('.modal-top button[aria-label=关闭]')
    await clickText('操作审计', '.admin-sidebar>button')
    await page.waitForSelector('tbody tr')
    checks.push('admin user state, logout presence, detail and audit')
  }
  assert.deepEqual(errors, [])
  console.log(
    JSON.stringify(
      { status: 'passed', checks, browserErrors: errors, screenshotDirectory: output },
      null,
      2,
    ),
  )
} catch (error) {
  await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {})
  console.error(error)
  process.exitCode = 1
} finally {
  await browser.close()
}
