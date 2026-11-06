# Invoice Administration Actions

Invoice rejection is available in the click-to-open More menu in both frontend
themes. Mark as Issued remains a direct primary action. Opening or dismissing the
menu must not change invoice status; only selecting Reject Application sends
status 3. Mark as Issued continues to send status 2.

The menu trigger and rejection item are disabled while a status request is in
flight. Existing visibility, permission checks and backend transitions are
unchanged. Existing translation keys are reused. No database changes are needed;
this is a frontend update and has not been deployed to production.

Verification: both frontend builds and changed-file lint pass. The development
branch retains 103 existing Select-related typecheck diagnostics, none in the
modified invoice component. The finance branch contains its existing Select type
fix and is checked separately after rebase.
Desktop and mobile browser checks in both themes verify that rejection is hidden
by default, opening/dismissing the menu sends no request, selecting rejection
sends status 3, and the direct issuance action sends status 2.
