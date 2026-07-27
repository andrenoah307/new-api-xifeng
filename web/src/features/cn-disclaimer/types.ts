export type CnDisclaimerResponse = {
  success: boolean
  enabled: boolean
  title: string
  content: string
  blocked_countries: string[]
  silence_minutes: number
  hash: string
}
