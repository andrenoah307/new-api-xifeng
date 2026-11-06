export function createFinanceMoney(unit: string, rate: string): {
  valid: boolean
  parse(value: string): number | null
  input(value: number): string
  format(value: number | string): string
}
