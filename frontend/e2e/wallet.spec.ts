import { test, expect, type Page, request } from '@playwright/test';

// These e2e tests run against the real built SPA + live Go backend (vite preview
// proxies /api -> backend). Each test creates uniquely-named wallets so repeated
// runs against a shared DB never collide. The new UI is a single-user interactive
// app: onboarding (name + initial balance) then 3 one-click actions.

const API = process.env.PUBLIC_API_BASE_URL || 'http://localhost:8080';

// Fixed nominals mirrored from App.svelte (major units).
const TOPUP = 50_000;
const SEND = 25_000;
const SPEND = 20_000;

function uniq(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
}

// Create a wallet straight through the API (used to seed "other users" so the
// Kirim button has a recipient). Returns the wallet id.
async function seedWallet(owner: string, initialMajor: number): Promise<string> {
  const ctx = await request.newContext();
  const res = await ctx.post(`${API}/api/v1/wallets`, {
    data: { owner, initial_balance: Math.round(initialMajor * 100) },
  });
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  await ctx.dispose();
  return body.data.id;
}

// Pick a recipient through the searchable combobox (Select2-style): type the
// name to filter, then click the matching option in the listbox.
async function pickRecipient(sendSection: ReturnType<Page['locator']>, recipient: string) {
  await sendSection.getByRole('combobox', { name: 'Cari penerima' }).fill(recipient);
  await sendSection.getByRole('option', { name: new RegExp(recipient) }).click();
}

// Complete the onboarding form and land on the main app.
async function onboard(page: Page, name: string, initialMajor: number) {
  await page.getByPlaceholder('Nama kamu').fill(name);
  await page.getByPlaceholder(/Saldo awal/).fill(String(initialMajor));
  await page.getByRole('button', { name: 'Masuk' }).click();
  await expect(page.getByRole('heading', { name: new RegExp(name) })).toBeVisible();
}

function balanceCard(page: Page) {
  return page.locator('div', { hasText: 'Saldo kamu' }).last();
}
function errorBanner(page: Page) {
  return page.locator('div.bg-red-950');
}

test.describe('E-Wallet interactive app', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Fresh context => no saved wallet => onboarding shows.
    await expect(page.getByRole('heading', { name: 'E-Wallet Demo' })).toBeVisible();
  });

  // ---- onboarding ------------------------------------------------------

  test('onboarding screen shows on first visit', async ({ page }) => {
    await expect(page.getByPlaceholder('Nama kamu')).toBeVisible();
    await expect(page.getByPlaceholder(/Saldo awal/)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Masuk' })).toBeVisible();
  });

  test('onboarding validation: empty name shows error', async ({ page }) => {
    await page.getByRole('button', { name: 'Masuk' }).click();
    await expect(page.getByText('Nama wajib diisi')).toBeVisible();
  });

  test('onboarding success shows balance card with formatted initial balance', async ({ page }) => {
    const name = uniq('budi');
    await onboard(page, name, 100_000);
    await expect(balanceCard(page)).toContainText('100.000,00');
  });

  test('onboarding with zero initial balance works', async ({ page }) => {
    const name = uniq('zero');
    await onboard(page, name, 0);
    await expect(balanceCard(page)).toContainText('0,00');
  });

  // ---- persistence -----------------------------------------------------

  test('wallet persists across reload (localStorage)', async ({ page }) => {
    const name = uniq('persist');
    await onboard(page, name, 50_000);
    await page.reload();
    // Should skip onboarding and go straight to the main app.
    await expect(page.getByRole('heading', { name: new RegExp(name) })).toBeVisible();
    await expect(balanceCard(page)).toContainText('50.000,00');
  });

  test('logout returns to onboarding and forgets the wallet', async ({ page }) => {
    const name = uniq('logout');
    await onboard(page, name, 10_000);
    await page.getByRole('button', { name: 'Ganti user' }).click();
    await expect(page.getByRole('heading', { name: 'E-Wallet Demo' })).toBeVisible();
    await page.reload();
    // Still onboarding after reload — wallet id was cleared.
    await expect(page.getByRole('heading', { name: 'E-Wallet Demo' })).toBeVisible();
  });

  // ---- one-click: Topup ------------------------------------------------

  test('Topup increases balance by the fixed amount', async ({ page }) => {
    const name = uniq('topup');
    await onboard(page, name, 0);
    await page.getByRole('button', { name: /Topup/ }).click();
    await expect(balanceCard(page)).toContainText(`${TOPUP.toLocaleString('id-ID')},00`);
  });

  test('Topup is reflected in the ledger', async ({ page }) => {
    const name = uniq('topupledg');
    await onboard(page, name, 0);
    await page.getByRole('button', { name: /Topup/ }).click();
    await expect(balanceCard(page)).toContainText('50.000,00');
    const ledger = page.locator('section', { hasText: 'Riwayat transaksi' });
    await expect(ledger.getByText('⬆️ Topup')).toBeVisible();
  });

  // ---- one-click: Belanja (withdraw) -----------------------------------

  test('Belanja decreases balance by the fixed amount', async ({ page }) => {
    const name = uniq('spend');
    await onboard(page, name, 100_000);
    await page.getByRole('button', { name: /Belanja/ }).click();
    // 100.000 - 20.000 = 80.000
    await expect(balanceCard(page)).toContainText('80.000,00');
  });

  test('Belanja with insufficient balance is blocked client-side', async ({ page }) => {
    const name = uniq('poor');
    await onboard(page, name, 1_000); // < SPEND
    await page.getByRole('button', { name: /Belanja/ }).click();
    // Toast warns; balance unchanged.
    await expect(page.getByText(/Saldo tidak cukup untuk belanja/)).toBeVisible();
    await expect(balanceCard(page)).toContainText('1.000,00');
  });

  // ---- one-click: Kirim (transfer to a chosen user) --------------------

  test('Kirim requires picking a recipient first (button disabled until selected)', async ({ page }) => {
    await seedWallet(uniq('recip'), 0); // ensure dropdown has at least one option
    const name = uniq('picker');
    await onboard(page, name, 100_000);
    const sendSection = page.locator('section', { hasText: 'Kirim saldo' });
    // Before selecting, the Kirim button is disabled.
    await expect(sendSection.getByRole('button', { name: /Kirim/ })).toBeDisabled();
  });

  test('Kirim recipient search filters the list as you type', async ({ page }) => {
    const wanted = uniq('alice');
    const other = uniq('zzznomatch');
    await seedWallet(wanted, 0);
    await seedWallet(other, 0);

    const name = uniq('searcher');
    await onboard(page, name, 100_000);

    const sendSection = page.locator('section', { hasText: 'Kirim saldo' });
    await sendSection.getByRole('combobox', { name: 'Cari penerima' }).fill(wanted);
    // Matching user is offered; the non-matching one is filtered out.
    await expect(sendSection.getByRole('option', { name: new RegExp(wanted) })).toBeVisible();
    await expect(sendSection.getByRole('option', { name: new RegExp(other) })).toHaveCount(0);
  });

  test('Kirim keeps the recipient selected after sending so you can spam transfers', async ({ page }) => {
    const recipient = uniq('spamrecv');
    const recipientId = await seedWallet(recipient, 0);

    const name = uniq('spammer');
    await onboard(page, name, 100_000);

    const sendSection = page.locator('section', { hasText: 'Kirim saldo' });
    await pickRecipient(sendSection, recipient);

    const kirim = sendSection.getByRole('button', { name: /Kirim/ });
    // First send.
    await kirim.click();
    await expect(balanceCard(page)).toContainText('75.000,00'); // 100k - 25k

    // Recipient stays selected (combobox keeps the name, button stays enabled).
    await expect(sendSection.getByRole('combobox', { name: 'Cari penerima' })).toHaveValue(recipient);
    await expect(kirim).toBeEnabled();

    // Second send without re-picking — proves spam works.
    await kirim.click();
    await expect(balanceCard(page)).toContainText('50.000,00'); // 75k - 25k

    // Recipient received both legs.
    const ctx = await request.newContext();
    const res = await ctx.get(`${API}/api/v1/wallets/${recipientId}/ledger`);
    const body = await res.json();
    await ctx.dispose();
    const credits = body.data.filter((e: { type: string }) => e.type === 'transfer_credit');
    expect(credits.length).toBe(2);
  });

  test('Kirim to a selected user moves funds and labels history with the recipient name', async ({ page }) => {
    const recipient = uniq('recv');
    const recipientId = await seedWallet(recipient, 0);

    const name = uniq('sender');
    await onboard(page, name, 100_000);

    const sendSection = page.locator('section', { hasText: 'Kirim saldo' });
    await pickRecipient(sendSection, recipient);
    await sendSection.getByRole('button', { name: /Kirim/ }).click();

    // Sender balance drops by SEND.
    await expect(balanceCard(page)).toContainText('75.000,00'); // 100k - 25k

    // History shows "Kirim ke <recipient>".
    const ledger = page.locator('section', { hasText: 'Riwayat transaksi' });
    await expect(ledger.getByText(`✈️ Kirim ke ${recipient}`)).toBeVisible();

    // Recipient really received it (verified via API) with the credit leg
    // carrying the sender's name as counterparty.
    const ctx = await request.newContext();
    const res = await ctx.get(`${API}/api/v1/wallets/${recipientId}/ledger`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    await ctx.dispose();
    const credit = body.data.find((e: { type: string }) => e.type === 'transfer_credit');
    expect(credit).toBeTruthy();
    expect(credit.amount).toBe(SEND * 100);     // minor units
    expect(credit.counterparty).toBe(name);     // recipient sees sender's name
  });

  test('recipient sees incoming transfer in their own history (realtime)', async ({ page, context }) => {
    // Sender created via API; recipient is the live browser user. A transfer from
    // the seeded sender should appear in the live user's history within the 2s poll.
    const recipient = uniq('liverecv');
    await onboard(page, recipient, 0);

    // Grab the live user's wallet id from localStorage.
    const recipientId = await page.evaluate(() => localStorage.getItem('ewallet_wallet_id'));
    expect(recipientId).toBeTruthy();

    // Seed a sender with funds, then transfer to the live recipient via API.
    const senderName = uniq('livesend');
    const senderId = await seedWallet(senderName, 100_000);
    const ctx = await request.newContext();
    const tr = await ctx.post(`${API}/api/v1/transfers`, {
      headers: { 'Idempotency-Key': `live-${Date.now()}` },
      data: { from_wallet_id: senderId, to_wallet_id: recipientId, amount: 25_000 * 100 },
    });
    expect(tr.ok()).toBeTruthy();
    await ctx.dispose();

    // Realtime poll should surface the incoming credit + sender name without reload.
    const ledger = page.locator('section', { hasText: 'Riwayat transaksi' });
    await expect(ledger.getByText(`⬇️ Terima dari ${senderName}`)).toBeVisible({ timeout: 8000 });
    await expect(balanceCard(page)).toContainText('25.000,00');
  });

  test('Kirim blocked when sender balance below send amount', async ({ page }) => {
    const recipient = uniq('target');
    await seedWallet(recipient, 0); // ensure a recipient exists
    const name = uniq('broke');
    await onboard(page, name, 10_000); // < SEND (25k)
    const sendSection = page.locator('section', { hasText: 'Kirim saldo' });
    await pickRecipient(sendSection, recipient);
    await sendSection.getByRole('button', { name: /Kirim/ }).click();
    await expect(page.getByText(/Saldo tidak cukup untuk kirim/)).toBeVisible();
    await expect(balanceCard(page)).toContainText('10.000,00');
  });

  // ---- realtime / version ----------------------------------------------

  test('balance-changing op bumps the wallet version', async ({ page }) => {
    const name = uniq('ver');
    await onboard(page, name, 0);
    await expect(balanceCard(page)).toContainText('v0');
    await page.getByRole('button', { name: /Topup/ }).click();
    await expect(balanceCard(page)).toContainText('50.000,00');
    await expect(balanceCard(page)).toContainText('v1');
  });

  test('multiple ops accumulate correctly (topup then belanja)', async ({ page }) => {
    const name = uniq('multi');
    await onboard(page, name, 0);
    await page.getByRole('button', { name: /Topup/ }).click();
    await expect(balanceCard(page)).toContainText('50.000,00');
    await page.getByRole('button', { name: /Belanja/ }).click();
    await expect(balanceCard(page)).toContainText('30.000,00'); // 50k - 20k
  });

  // ---- empty state -----------------------------------------------------

  test('fresh wallet shows empty ledger hint', async ({ page }) => {
    const name = uniq('fresh');
    await onboard(page, name, 0);
    await expect(page.getByText(/Belum ada transaksi/)).toBeVisible();
  });
});
