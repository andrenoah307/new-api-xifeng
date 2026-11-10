export type RefundDiagnostic = {
  code: string;
  ticket_id?: number;
  order_id?: number;
  amounts?: Record<string, string>;
};

export function refundDiagnosticLines(
  diagnostic: RefundDiagnostic,
  t: (key: string, values?: Record<string, string>) => string,
  exact?: boolean,
): string[];
