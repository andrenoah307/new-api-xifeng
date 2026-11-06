# Invoice Administration Actions

Both frontend themes display Reject Application directly. Clicking it opens a
confirmation dialog showing the company name; opening, cancelling or dismissing
that dialog must not change invoice status. Only confirming rejection sends
status 3. Mark as Issued remains a direct primary action and sends status 2.

The reject button and confirmation are disabled while a status request is in
flight. Confirmation is bound to the displayed invoice ID. Existing visibility,
permission checks and backend transitions are
unchanged. Existing translation keys are reused. No database changes are needed;
this is a frontend update and has not been deployed to production.

Verification: both frontend builds and changed-file lint pass. The development
branch retains 103 existing Select-related typecheck diagnostics, none in the
modified invoice component. The finance branch contains its existing Select type
fix and is checked separately after rebase.
Desktop and mobile browser checks cover direct rejection, cancellation without
a request, explicit confirmation sending status 3, and direct issuance sending
status 2. The former More-menu presentation was replaced at the user's request.
