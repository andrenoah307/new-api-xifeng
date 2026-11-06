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

Finance rebase verification (2026-11-06): all 36 commits from 98eaf509b were
replayed onto dev d11eee7c5. Two invoice-detail conflict resolutions retain the
finance payment display and unpaid-fee issuance guard alongside the new menu.
Before this documentation update, the aggregate difference from the old finance
tip consists only of this document and the two action components; backend,
configuration, locale and other finance files are identical. Both builds, scoped
lint and the complete Default typecheck pass. Both themes pass desktop/mobile
action checks, including disabled issuance with unpaid tax and enabled issuance
after payment. No production deployment was performed.
