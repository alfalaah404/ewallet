// API client. Base URL is build-time configurable for public-preview tunnels;
// defaults to same-origin '/api/v1' (works behind a reverse proxy) or localhost in dev.
const RAW = import.meta.env.PUBLIC_API_BASE_URL || '';
export const API_BASE = RAW ? `${RAW}/api/v1` : '/api/v1';

export interface Wallet {
  id: string;
  owner: string;
  balance: number;
  version: number;
}

export interface LedgerEntry {
  type: string;
  amount: number;
  balance_after: number;
  created_at: string;
  counterparty: string; // other party's owner name for transfers; '' otherwise
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...opts,
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = body?.error?.message || `request failed (${res.status})`;
    throw new Error(msg);
  }
  return body.data as T;
}

function idemKey(): string {
  return (crypto.randomUUID?.() ?? `${Date.now()}-${Math.random()}`);
}

export const api = {
  listWallets: () => req<Wallet[]>('/wallets'),
  getWallet: (id: string) => req<Wallet>(`/wallets/${id}`),
  createWallet: (owner: string, initial_balance: number) =>
    req<Wallet>('/wallets', { method: 'POST', body: JSON.stringify({ owner, initial_balance }) }),
  deposit: (id: string, amount: number) =>
    req<Wallet>(`/wallets/${id}/deposit`, {
      method: 'POST', headers: { 'Idempotency-Key': idemKey() }, body: JSON.stringify({ amount }),
    }),
  withdraw: (id: string, amount: number) =>
    req<Wallet>(`/wallets/${id}/withdraw`, {
      method: 'POST', headers: { 'Idempotency-Key': idemKey() }, body: JSON.stringify({ amount }),
    }),
  transfer: (from_wallet_id: string, to_wallet_id: string, amount: number) =>
    req<unknown>('/transfers', {
      method: 'POST', headers: { 'Idempotency-Key': idemKey() },
      body: JSON.stringify({ from_wallet_id, to_wallet_id, amount }),
    }),
  ledger: (id: string) => req<LedgerEntry[]>(`/wallets/${id}/ledger`),
};

// Money is stored as integer minor units. Display as major units with 2 decimals.
export function fmt(minor: number): string {
  return (minor / 100).toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}
