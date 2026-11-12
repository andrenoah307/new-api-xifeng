import { z } from 'zod'

export const upstreamErrorSchema = z.object({
  client: z.string().max(300),
  server: z.string().max(300),
})

export type UpstreamErrorValues = z.infer<typeof upstreamErrorSchema>
