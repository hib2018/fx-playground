import { expect, test } from '@playwright/test'

test('board, mock commands and real WASM terminal lifecycle without model requests', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  let generations = 0
  page.on('request', request => {
    if (request.url().includes('/api/gateway') && request.method() === 'POST') generations++
  })
  await page.goto('/')
  await expect(page.locator('#task-count')).toHaveText('3')
  await expect(page.locator('#connection')).toHaveText('Not connected')
  await expect(page.locator('#todo .task-card')).toHaveCount(1)
  await page.locator('#todo .task-card').click()
  await expect(page.locator('#doing .task-card')).toHaveCount(2)
  await page.getByRole('button', { name: 'モックを試す' }).click()
  await expect(page.locator('#runtime-status')).toContainText('モック結果を表示しました')
  await expect(page.locator('#terminal')).toContainText('mock/workspace')
  await page.getByRole('button', { name: 'fxを起動' }).click()
  await expect(page.locator('#runtime-status')).toContainText('fx起動済み', { timeout: 45_000 })
  await page.getByRole('button', { name: '/help', exact: true }).click()
  await expect(page.locator('#terminal')).toContainText('/clear', { timeout: 15_000 })
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(page.locator('#runtime-status')).toContainText('exit 130')
  await expect(page.getByRole('button', { name: 'fxを起動' })).toBeEnabled()
  await page.getByRole('button', { name: 'fxを起動' }).click()
  await expect(page.locator('#runtime-status')).toContainText('fx起動済み', { timeout: 45_000 })
  await page.getByRole('button', { name: '停止', exact: true }).click()
  await expect(page.locator('#runtime-status')).toContainText('exit 130')
  expect(generations).toBe(0)
  expect(errors).toEqual([])
})

test('mobile layout fits viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await expect(page.locator('#task-count')).toHaveText('3')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
