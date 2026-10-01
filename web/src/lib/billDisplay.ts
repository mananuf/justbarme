import type { Bill, Customer, Table } from '../api/tabs';

export function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

// billLabel names a bill for display -- a real table's label, a named
// customer's name, or "Walk-in" when neither is attached (the common case
// for a sale recorded with both of the unified Sell flow's pickers left
// at their defaults, docs/PHASE_UNIFIED_SELL_BILLS.md). Shared by the
// Bills list and the bill detail page so a tab reads the same way in both
// places.
export function billLabel(bill: Bill, tables: Table[], customers: Customer[]): string {
  if (bill.tableId) return tables.find((t) => t.id === bill.tableId)?.label ?? 'Table';
  if (bill.customerId)
    return customers.find((c) => c.id === bill.customerId)?.name ?? 'Customer tab';
  return 'Walk-in';
}

export function statusLabel(status: Bill['status']): string {
  switch (status) {
    case 'open':
      return 'Open';
    case 'closed_unpaid':
      return 'Closed — balance due';
    case 'settled':
      return 'Settled';
    case 'void':
      return 'Void';
  }
}
