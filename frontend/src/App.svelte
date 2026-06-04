<script lang="ts">
  import { api, fmt, type Wallet, type LedgerEntry } from './lib/api';

  // Fixed nominal (minor units = x100). 1-click simulation.
  const TOPUP = 50_000 * 100;   // +50.000
  const SEND  = 25_000 * 100;   // 25.000 ke user acak
  const SPEND = 20_000 * 100;   // -20.000

  const STORAGE_KEY = 'ewallet_wallet_id';

  let me = $state<Wallet | null>(null);
  let ledger = $state<LedgerEntry[]>([]);
  let others = $state<Wallet[]>([]);
  let sendTo = $state('');       // selected recipient wallet id for Kirim
  let sendQuery = $state('');     // search text in the recipient combobox
  let sendOpen = $state(false);   // combobox dropdown open state
  let error = $state('');
  let toast = $state('');
  let busy = $state(false);     // locks the 3 action buttons during a request
  let booting = $state(true);

  // onboarding form
  let newOwner = $state('');
  let newBalance = $state(0);

  let pollTimer: ReturnType<typeof setInterval> | null = null;

  function flash(msg: string) {
    toast = msg;
    setTimeout(() => { if (toast === msg) toast = ''; }, 2500);
  }

  // Pull my wallet + ledger + other wallets. Used by both manual actions and the
  // realtime poll, so the screen reflects what other concurrent demo users do.
  async function sync() {
    if (!me) return;
    try {
      const [fresh, all, lg] = await Promise.all([
        api.getWallet(me.id),
        api.listWallets(),
        api.ledger(me.id),
      ]);
      me = fresh;
      others = all.filter((w) => w.id !== fresh.id);
      ledger = lg;
      error = '';
    } catch (e) {
      // wallet vanished (db reset) => kick back to onboarding
      const msg = (e as Error).message;
      if (msg.includes('not found')) { logout(); return; }
      error = msg;
    }
  }

  function startPolling() {
    stopPolling();
    pollTimer = setInterval(sync, 2000); // realtime-ish refresh
  }
  function stopPolling() {
    if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }

  async function boot() {
    booting = true;
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
      try {
        me = await api.getWallet(saved);
        await sync();
        startPolling();
      } catch {
        localStorage.removeItem(STORAGE_KEY);
        me = null;
      }
    }
    booting = false;
  }

  async function onboard() {
    if (!newOwner.trim()) { error = 'Nama wajib diisi'; return; }
    if (newBalance < 0)  { error = 'Saldo awal tidak boleh negatif'; return; }
    busy = true;
    try {
      const w = await api.createWallet(newOwner.trim(), Math.round(newBalance * 100));
      localStorage.setItem(STORAGE_KEY, w.id);
      me = w;
      await sync();
      startPolling();
      flash(`Selamat datang, ${w.owner}! 🎉`);
    } catch (e) { error = (e as Error).message; }
    finally { busy = false; }
  }

  function logout() {
    stopPolling();
    localStorage.removeItem(STORAGE_KEY);
    me = null; ledger = []; others = []; sendTo = ''; sendQuery = ''; sendOpen = false;
    newOwner = ''; newBalance = 0;
  }

  // ---- 3 one-click features ----

  async function topup() {
    if (!me || busy) return;
    busy = true;
    try { await api.deposit(me.id, TOPUP); await sync(); flash(`Topup +${fmt(TOPUP)} ✅`); }
    catch (e) { error = (e as Error).message; }
    finally { busy = false; }
  }

  async function sendMoney() {
    if (!me || busy) return;
    if (others.length === 0) { flash('Belum ada user lain untuk dikirim 🤷'); return; }
    if (!sendTo)             { flash('Pilih dulu penerimanya 👈'); return; }
    if (me.balance < SEND)   { flash('Saldo tidak cukup untuk kirim 💸'); return; }
    const target = others.find((w) => w.id === sendTo);
    if (!target) { flash('Penerima tidak ditemukan, pilih ulang 🤔'); sendTo = ''; return; }
    busy = true;
    try {
      await api.transfer(me.id, target.id, SEND);
      await sync();
      flash(`Kirim ${fmt(SEND)} ke ${target.owner} ✈️`);
      // Keep recipient selected so you can spam transfers to the same user.
    } catch (e) { error = (e as Error).message; }
    finally { busy = false; }
  }

  async function spend() {
    if (!me || busy) return;
    if (me.balance < SPEND) { flash('Saldo tidak cukup untuk belanja 🛒'); return; }
    busy = true;
    try { await api.withdraw(me.id, SPEND); await sync(); flash(`Belanja -${fmt(SPEND)} 🛍️`); }
    catch (e) { error = (e as Error).message; }
    finally { busy = false; }
  }

  // Human label for a ledger row, including who the money went to / came from.
  function entryLabel(e: LedgerEntry): string {
    switch (e.type) {
      case 'deposit':         return '⬆️ Topup';
      case 'withdraw':        return '🛍️ Belanja';
      case 'transfer_debit':  return `✈️ Kirim ke ${e.counterparty || '—'}`;
      case 'transfer_credit': return `⬇️ Terima dari ${e.counterparty || '—'}`;
      default:                return e.type;
    }
  }
  function isCredit(t: string): boolean {
    return t === 'deposit' || t === 'transfer_credit';
  }

  // ---- recipient combobox (Select2-style search) ----
  // Filter other users by name (case-insensitive). Cap the visible list so a
  // huge demo crowd doesn't render hundreds of rows at once.
  let filteredOthers = $derived(
    (() => {
      const q = sendQuery.trim().toLowerCase();
      const list = q ? others.filter((w) => w.owner.toLowerCase().includes(q)) : others;
      return list.slice(0, 50);
    })(),
  );
  let selectedRecipient = $derived(others.find((w) => w.id === sendTo) ?? null);

  function pickRecipient(w: Wallet) {
    sendTo = w.id;
    sendQuery = w.owner;
    sendOpen = false;
  }
  function clearRecipient() {
    sendTo = '';
    sendQuery = '';
    sendOpen = true;
  }
  function onSendInput() {
    // Typing invalidates a prior selection until they pick again.
    sendTo = '';
    sendOpen = true;
  }

  $effect(() => { boot(); return () => stopPolling(); });
</script>

<main class="min-h-screen bg-slate-950 text-slate-100">
  {#if toast}
    <div class="fixed top-4 left-1/2 -translate-x-1/2 z-50 px-4 py-2 rounded-xl
                bg-indigo-600 shadow-lg text-sm font-medium animate-pulse">{toast}</div>
  {/if}

  {#if booting}
    <div class="min-h-screen grid place-items-center text-slate-500">Memuat…</div>

  {:else if !me}
    <!-- ONBOARDING -->
    <div class="min-h-screen grid place-items-center p-6">
      <div class="w-full max-w-sm bg-slate-900 rounded-2xl p-7 border border-slate-800 space-y-4">
        <div class="text-center space-y-1">
          <div class="text-4xl">💳</div>
          <h1 class="text-xl font-bold">E-Wallet Demo</h1>
          <p class="text-slate-400 text-sm">Masukkan nama & saldo awal untuk mulai</p>
        </div>
        {#if error}
          <div class="rounded-lg bg-red-950 border border-red-800 text-red-200 px-3 py-2 text-sm">{error}</div>
        {/if}
        <input class="w-full bg-slate-800 rounded-lg px-3 py-2.5 outline-none focus:ring-2 ring-indigo-500"
          placeholder="Nama kamu" bind:value={newOwner} onkeydown={(e) => e.key === 'Enter' && onboard()} />
        <input type="number" min="0" step="1000"
          class="w-full bg-slate-800 rounded-lg px-3 py-2.5 outline-none focus:ring-2 ring-indigo-500"
          placeholder="Saldo awal (mis. 100000)" bind:value={newBalance}
          onkeydown={(e) => e.key === 'Enter' && onboard()} />
        <button class="w-full py-2.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 font-semibold disabled:opacity-50"
          disabled={busy} onclick={onboard}>{busy ? 'Membuat…' : 'Masuk'}</button>
      </div>
    </div>

  {:else}
    <!-- MAIN APP -->
    <div class="max-w-md mx-auto p-5 space-y-5">
      <header class="flex items-center justify-between">
        <div>
          <p class="text-slate-400 text-xs">Halo,</p>
          <h1 class="text-lg font-bold">{me.owner} 👋</h1>
        </div>
        <button class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs" onclick={logout}>
          Ganti user
        </button>
      </header>

      {#if error}
        <div class="rounded-lg bg-red-950 border border-red-800 text-red-200 px-3 py-2 text-sm">{error}</div>
      {/if}

      <!-- Balance card -->
      <div class="rounded-2xl bg-gradient-to-br from-indigo-600 to-violet-700 p-6 shadow-xl">
        <p class="text-indigo-200 text-sm">Saldo kamu</p>
        <p class="text-4xl font-bold tabular-nums mt-1">Rp {fmt(me.balance)}</p>
        <div class="flex items-center gap-2 mt-3 text-indigo-200 text-xs">
          <span class="inline-block w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          realtime · v{me.version} · {others.length} user lain online
        </div>
      </div>

      <!-- Topup & Belanja: pure 1-click -->
      <div class="grid grid-cols-2 gap-3">
        <button class="flex flex-col items-center gap-1.5 py-4 rounded-xl bg-emerald-600 hover:bg-emerald-500
                       active:scale-95 transition disabled:opacity-50" disabled={busy} onclick={topup}>
          <span class="text-2xl">⬆️</span>
          <span class="text-xs font-semibold">Topup</span>
          <span class="text-[10px] text-emerald-100">+{fmt(TOPUP)}</span>
        </button>
        <button class="flex flex-col items-center gap-1.5 py-4 rounded-xl bg-rose-600 hover:bg-rose-500
                       active:scale-95 transition disabled:opacity-50" disabled={busy} onclick={spend}>
          <span class="text-2xl">🛍️</span>
          <span class="text-xs font-semibold">Belanja</span>
          <span class="text-[10px] text-rose-100">-{fmt(SPEND)}</span>
        </button>
      </div>

      <!-- Kirim: pick a recipient first, then send -->
      <section class="bg-slate-900 rounded-xl p-4 border border-slate-800 space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="font-semibold text-sm">✈️ Kirim saldo</h2>
          <span class="text-xs text-slate-400">{fmt(SEND)} per kirim</span>
        </div>
        {#if others.length === 0}
          <p class="text-slate-500 text-sm">Belum ada user lain. Tunggu user lain bergabung 🙌</p>
        {:else}
          <!-- Searchable recipient combobox (Select2-style) -->
          <div class="relative">
            {#if sendOpen}
              <!-- click-outside backdrop to close the list -->
              <button type="button" tabindex="-1" aria-hidden="true"
                class="fixed inset-0 z-10 cursor-default"
                onclick={() => (sendOpen = false)}></button>
            {/if}
            <div class="flex items-center gap-2 relative z-20">
              <input
                type="text"
                role="combobox"
                aria-expanded={sendOpen}
                aria-controls="recipient-listbox"
                aria-label="Cari penerima"
                autocomplete="off"
                placeholder="Cari nama penerima…"
                class="w-full bg-slate-800 rounded-lg px-3 py-2.5 outline-none focus:ring-2 ring-sky-500
                       {sendTo ? 'ring-1 ring-sky-600' : ''}"
                bind:value={sendQuery}
                oninput={onSendInput}
                onfocus={() => (sendOpen = true)} />
              {#if sendQuery}
                <button type="button" aria-label="Hapus pilihan"
                  class="shrink-0 px-2.5 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-400"
                  onclick={clearRecipient}>✕</button>
              {/if}
            </div>

            {#if sendOpen}
              <ul id="recipient-listbox" class="absolute z-20 mt-1 w-full max-h-56 overflow-auto rounded-lg border border-slate-700
                         bg-slate-800 shadow-xl divide-y divide-slate-700/60" role="listbox">
                {#each filteredOthers as w (w.id)}
                  <li role="option" aria-selected={sendTo === w.id}>
                    <button type="button"
                      class="w-full text-left px-3 py-2.5 hover:bg-slate-700/70 flex justify-between gap-3
                             {sendTo === w.id ? 'bg-sky-900/40' : ''}"
                      onclick={() => pickRecipient(w)}>
                      <span class="truncate">{w.owner}</span>
                      <span class="shrink-0 text-slate-400 tabular-nums text-sm">Rp {fmt(w.balance)}</span>
                    </button>
                  </li>
                {:else}
                  <li class="px-3 py-2.5 text-slate-500 text-sm">Tidak ada user cocok “{sendQuery}”</li>
                {/each}
              </ul>
            {/if}
          </div>

          {#if selectedRecipient}
            <p class="text-xs text-sky-300">Penerima: <span class="font-semibold">{selectedRecipient.owner}</span></p>
          {/if}

          <button class="w-full py-2.5 rounded-lg bg-sky-600 hover:bg-sky-500 font-semibold
                         active:scale-95 transition disabled:opacity-50"
            disabled={busy || !sendTo} onclick={sendMoney}>
            {busy ? 'Mengirim…' : `Kirim ${fmt(SEND)}`}
          </button>
        {/if}
      </section>

      <!-- Ledger -->
      <section class="bg-slate-900 rounded-xl p-4 border border-slate-800">
        <h2 class="font-semibold text-sm mb-2">Riwayat transaksi</h2>
        {#if ledger.length === 0}
          <p class="text-slate-500 text-sm py-2">Belum ada transaksi. Coba salah satu tombol di atas 👆</p>
        {:else}
          <div class="space-y-1 text-sm max-h-72 overflow-auto">
            {#each ledger as e}
              <div class="flex justify-between items-center gap-2 border-b border-slate-800/70 py-1.5">
                <span class="text-slate-300 truncate flex-1">{entryLabel(e)}</span>
                <span class="tabular-nums shrink-0 {isCredit(e.type) ? 'text-emerald-400' : 'text-rose-400'}">
                  {isCredit(e.type) ? '+' : '-'}{fmt(e.amount)}
                </span>
                <span class="tabular-nums text-slate-500 text-xs shrink-0">Rp {fmt(e.balance_after)}</span>
                <span class="text-slate-600 text-xs shrink-0">{new Date(e.created_at).toLocaleTimeString('id-ID')}</span>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <p class="text-center text-slate-600 text-xs">
        Go/Fiber v3 · PostgreSQL · race-safe · idempotent · realtime polling 2s
      </p>
    </div>
  {/if}
</main>
