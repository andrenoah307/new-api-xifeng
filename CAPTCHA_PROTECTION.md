# Captcha protection

The authentication settings page in both themes supports Cloudflare Turnstile
and Tencent Cloud Captcha. Each action can select either provider or remain off.
Only an enabled root account, checked against the database on every settings
request, can read or change this configuration.

## Upstream and module boundaries

Checked `upstream/main` at `ba5f7367797566175a6cd97a748d430af3602a01`:
there is no Tencent Captcha implementation to reuse. Its authentication refactor
is not required for this module. Keep these boundaries when merging upstream:

- `setting/captcha_setting`: policy, validation and legacy migration.
- `service/captcha`: provider verification and single-use challenge/proof storage.
- `middleware/captcha.go`: enforcement before business effects.
- `controller/captcha.go`, `model/captcha.go`, `router/captcha-router.go`: isolated
  settings/verification endpoints and concurrent settings updates.
- `web/shared`: lazy provider runtime, explicit-challenge replay and field metadata.
- Each theme owns its settings controls. Existing page forms use their normal
  Axios instance; they do not individually implement provider verification.

`TurnstileCheck` remains as a routing compatibility hook. It no longer accepts a
persistent session flag as proof. Routes added upstream must explicitly select
a known protection action if captcha is appropriate.

## Credentials

Prepare these Tencent values in the Tencent consoles, then enter them through
the root-only settings page. Do not put real credentials in a commit or chat.

| Setting | Source | Exposure |
| --- | --- | --- |
| CaptchaAppId | Captcha console, verification application | Public application identifier |
| AppSecretKey | The same captcha application | Server only |
| CAM SecretId | Dedicated CAM API credential | Server only |
| CAM SecretKey | The matching CAM API credential | Server only |

Authorize only the necessary `captcha:DescribeCaptchaResult` API access for the
CAM identity. AppSecretKey is not the CAM SecretKey. Match the CaptchaAppId and
AppSecretKey from the same application and configure the intended domain in the
Tencent console. The backend requires outbound HTTPS to `captcha.tencentcloudapi.com`;
browsers require the official Tencent captcha script and its challenge resources.

Turnstile uses its own Site Key and Secret Key, with the intended site hostname
configured in Cloudflare. CSP/network policies must also permit the relevant
provider's script, frames and challenge resources.

An optional **CaptchaAppId authentication** switch generates short-lived
`aidEncrypted` using Tencent's AES-256-GCM envelope, with `aidEncryptedType=gcm`.
Coordinate this switch with CaptchaAppId forced authentication in Tencent's
console. Do not enable the console requirement before deploying a client that
sends the encrypted value. This option helps limit AppID misuse; it does not
replace server-side ticket verification.

Secret inputs are blank on reads. Blank preserves the stored value; the explicit
clear checkbox deletes it. Configuration writes validate the complete result.
The document is stored in the existing options table as `CaptchaConfigSecret`,
excluded from generic option reads and blocked from generic option writes.
It has the same database-at-rest trust boundary as existing system credentials;
this change does not introduce database encryption or a secret vault.

## Protected actions

| Action | Business endpoints |
| --- | --- |
| Password login | `POST /api/user/login` |
| Registration | `POST /api/user/register` |
| Email verification code | `GET /api/verification` |
| Password reset email | `GET /api/reset_password` |
| Password reset | `POST /api/user/reset` |
| Discount codes | `/api/user/discount_code/validate`, plus Epay, Stripe, Creem, Waffo and Waffo Pancake amount/payment endpoints when a discount code is present |
| Redemption codes | `POST /api/user/topup` |
| Ticket submission | `POST /api/ticket/`, `/api/ticket/invoice/`, `/api/ticket/refund/` |
| Ticket replies | `POST /api/ticket/self/:id/message` |
| Ticket attachments | `POST /api/ticket/attachment` |
| Daily check-in | `POST /api/user/checkin` |

Payment callbacks, OAuth callbacks, passkey authentication and administrator ticket
operations are not captcha challenges. The payment amount/order gates apply only
when a code is submitted. Captcha does not replace authentication, reset-token
entropy/expiry, email throttling, payment validation, or attachment validation.

## Request protocol and failure behavior

1. Before business execution, a protected request without a proof receives HTTP
   200 with `success:false`, `code:captcha_required`, and public challenge data.
   No mail, payment, redemption or ticket operation has run at this point.
2. The shared Axios interceptor lazily loads the selected provider and obtains a
   ticket. It submits `{id,ticket,randstr}` to `POST /api/captcha/verify`.
3. The server atomically consumes the challenge, verifies the ticket with the
   provider and returns a short-lived opaque proof. Tencent verification requires
   `CaptchaType=9`, `CaptchaCode=1`, and no reported nonzero risk level. Disaster
   tickets beginning with `trerror` are rejected, even if the browser reports success.
4. The interceptor replays the intercepted request once with `X-Captcha-Proof`.
   The server atomically consumes the proof before allowing business execution.

Challenge and proof records expire after two minutes. They bind the configured
action, a random HttpOnly browser cookie, authenticated user ID (where applicable),
method, host, path, query, request body hash and configuration fingerprint. JSON bodies
are bounded at 64 KiB while protection is enabled. Multipart attachment proofs
bind action, browser, user and endpoint, but not file bytes or multipart boundaries;
the original FormData is replayed and normal upload validation still applies.

Cancellation, network failures, provider failures and ordinary business failures
never trigger an automatic business replay. A failed/repeated proof uses HTTP 200,
`success:false`, `code:captcha_failed`; provider/storage/configuration failures
use HTTP 503 and `code:captcha_unavailable`. Existing route rate limits remain;
captcha admission is additionally limited to 60 requests/IP/minute including the
proof replay, and verification to 30/IP/minute. Those limits use the existing
rate-limit infrastructure and its HTTP error responses.

Settings changes use a database compare-and-swap with row locking, so a stale
editor cannot overwrite another node's credential rotation. Successful updates
emit the `captcha.settings_update` management audit event without secret values.
Runtime settings propagate through the existing options synchronization interval;
revocation is effective on each node after it observes the update, not instantly
across the cluster. A configuration change observed during provider verification
prevents issuing a proof for the old configuration.

## Deployment and rollback

- Deploy backend and both frontend bundles together. `/api/status` reports the
  old `turnstile_check` flag as false because the unified request interceptor owns
  challenges. Old clients without that interceptor cannot complete the new flow.
- With no new configuration saved, existing enabled Turnstile settings migrate
  in memory to login, registration, email verification, reset email and check-in.
  All newly introduced actions remain off. Saving the new page makes its policy
  authoritative; legacy Turnstile option changes no longer affect it.
- Configure credentials and choose actions before enabling protection. Validate
  on a test application/domain before enabling production actions. This work
  does not enable protection or change credentials on any production instance.
- Multiple application nodes must share the same Redis instance/database and
  settings. Proof consumption uses atomic Lua operations. Configured Redis
  outages fail closed and never fall back to local memory.
- Without Redis, storage is bounded process-local memory (10,000 combined
  challenges/proofs). Use a single instance or sticky routing; cross-node
  verification is not supported in this mode. Restarting drops pending proofs.
- Both stores reject new entries at capacity. Verification consumes its challenge
  before contacting the provider, limiting duplicate paid verification calls.
- To disable, use the root settings page. If login itself is inaccessible due to
  a provider outage, use an already authenticated root session. Emergency offline
  recovery requires an operator to change the stored configuration and refresh
  node options through the normal deployment procedure; there is no public bypass.
- A binary rollback restores old Turnstile behavior from the legacy options;
  the new Tencent policy is not understood by the old binary. Review the legacy
  options before rollback, and roll back frontend/backend together.

## Verification

Automated tests cover provider success/rejection/malformed replies/disaster tickets,
one-use memory and Redis proofs, expiry/outage handling, request/browser/user
binding, fresh root authorization, secret preservation/clearing, stale writes,
generic option endpoint bypasses, actual router enforcement and client replay
boundaries. Browser checks use provider/API fixtures for both themes and desktop/
mobile layouts. Live Tencent verification requires the prepared application and
CAM credentials; mock-provider tests do not prove those credentials/domain rules.

Official references:

- [Web integration](https://cloud.tencent.com/document/product/1110/36841)
- [DescribeCaptchaResult](https://cloud.tencent.com/document/api/1110/36926)
- [Server SDK integration](https://cloud.tencent.com/document/product/1110/75489)
- [CaptchaAppId authentication](https://cloud.tencent.com/document/product/1110/128489)
