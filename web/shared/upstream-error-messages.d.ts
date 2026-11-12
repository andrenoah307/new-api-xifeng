export type UpstreamErrorMessages = {
  client: string;
  server: string;
};
export const DEFAULT_UPSTREAM_ERROR_MESSAGES: UpstreamErrorMessages;
export function parseUpstreamErrorMessages(raw?: string): UpstreamErrorMessages;
