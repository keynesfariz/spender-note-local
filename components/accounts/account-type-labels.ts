import type { AccountType } from "@/lib/api/types.generated"

export const ACCOUNT_TYPE_LABELS: Record<AccountType, string> = {
  credit_card: "Credit Card",
  debit_card: "Debit Card",
  digital_wallet: "Digital Wallet",
}
