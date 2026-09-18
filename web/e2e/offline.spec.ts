import { expect, test } from '@playwright/test';

// Aborting every /api/v1 request (rather than navigator.onLine, which is
// only a hint per src/hooks/useConnectivity.ts) is what actually makes
// fetch() throw the same way it would on a real dropped connection --
// this is what apiRequest's OfflineError path is built to catch.
test('shows a clear offline message when an activity needs a connection', async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => route.abort('internetdisconnected'));

  await page.goto('/login');
  await page.getByLabel(/email or whatsapp number/i).fill('owner@example.com');
  await page.getByLabel('Password').fill('correcthorsebattery');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page.getByRole('alert')).toHaveText(
    "You're offline. This needs an internet connection to work.",
  );
});
