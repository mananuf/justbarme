import { expect, test } from '@playwright/test';

test('renders the installable application shell', async ({ page }) => {
  await page.route('**/api/v1/health/ready', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: { status: 'ready', version: 'e2e' } }),
    });
  });
  await page.goto('/');
  await expect(page.getByRole('link', { name: 'justbarme home' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /run the shift/i })).toBeVisible();
  await expect(page.getByRole('status')).toContainText('Connected');
});
