# Pressure cooling recovery

Pressure cooling has its own recovery worker. The ordinary automatic-enable
switch and channel test schedule do not control this worker. Reaching the
configured consecutive cooldown limit still suspends the channel for manual
intervention; this change does not alter thresholds, timing or suspension policy.

## Recovery correctness

Revision-checked observation writes prevent an old observation from replacing a
new cooldown, but do not validate the channel snapshot used by the observer.
A request can hold an enabled channel snapshot and read a newer `cool` marker.
It must not infer manual recovery from that combination and replace `cool` with
`obs`, even when its marker revision matches.

Requests now leave `cool` markers intact. The recovery worker reads the database
status, including when reconciling a manual enable, instead of trusting the routing
cache. Failed database reads retain the marker.

Recovery from automatic disable uses a conditional, locked transaction. Only
status and status metadata are updated, and channel abilities are enabled in the
same transaction. Credentials, configuration and counters are untouched. Manual
disable is not converted to enabled. The routing cache changes only after commit.
Database or ability-update failure retains the cooldown and attempts a
revision-checked TTL refresh so the next recovery pass can retry. An unconfirmed
write never produces a success notification. Observation completion is also
revision-checked so it cannot overwrite a newer marker.

## Diagnostics and limits

Private application logs include `pressure cooling transition` and
`pressure cooling recovery`, channel ID, cooldown deadline, consecutive count,
and fixed result categories. They do not include credentials or request bodies.
The transition log describes the cooldown decision; it is not proof of a
cross-store commit. Existing store-error logs must be considered alongside it.
Manual-disable reconciliation does not send an automatic-recovery notification.

There is no schema migration or change to relay selection, affinity, billing,
or the ordinary auto-disable policy. The additional database read is per cooling
channel per recovery pass, not per relay request.

Redis and SQL are not a distributed transaction. Missing/expired historical
markers cannot safely be reconstructed from status alone: status 3 can also
come from other automatic-disable mechanisms. This patch does not bulk-enable
such channels or override deliberate suspension. Redis outages, concurrent
multi-node transitions and notification delivery remain separate operational
boundaries; deploying a fix does not establish the unique cause of an older
incident without its state history.

Regression tests cover stale enabled routing cache with an actual SQL disable,
failed recovery followed by a successful retry, ability-write rollback, manual
status preservation, and the earlier stale-observation race.

## Administrator usage logs

Automatic disable/enable and pressure-cooling transitions now write management
logs, searchable by channel ID in the administrator usage log view. Events include
cooldown decisions, recovery, suspension requiring manual intervention, disabled
configuration, preserved manual status, and failed recovery. Details include the
reason, effective thresholds, consecutive count and limit, deadlines, and the
observed results of state/status writes. A cooldown decision is not a guarantee
that both Redis and SQL succeeded; inspect `marker_saved` and `status_updated`.

Records belong to system user/token zero, so authenticated users' self, token,
and export queries do not return them. Structured details live in `admin_info`;
the content also contains a readable fallback for both existing frontend themes.
Ordinary disable reasons redact the current credential and common credential
patterns, normalize whitespace, and truncate to 800 characters. No request body
or credential object is attached. Identical recovery failures are limited to one
record per channel/reason every five minutes per process. Audit persistence is
best effort through the existing log queue; failures do not block recovery.

This does not backfill historical events. Group-only suspension does not set the
channel's automatic-disable status, so the new indicators use runtime state.
Tests cover administrator filtering/export, exclusion from self/token queries,
credential masking, successful recovery, failed-write/retry events and throttling.

## Per-channel suspension limit and visibility

Both editors support `max_consecutive_cooldowns` (integer 1 through 100) in the
existing channel settings JSON. Omitted/null/blank inherits the global limit;
the system default is 5, not a promise that a customized global value is 5.
Zero, negative, fractional and out-of-range inputs are rejected. Existing
channels inherit their previous policy. Changing the limit does not resume an
already suspended channel. No schema migration is needed.

Both channel pages show an alert listing suspended channel IDs and counts/limits,
independent of table/card mode and hidden columns. Per-channel runtime details
show suspension and affected groups even when only FRT cooling is configured.
Indicators follow the existing runtime refresh and require an available state
marker; expired/lost historical markers cannot be reconstructed by this patch.
The management log preserves the event history independently of runtime TTL.

Suspension notifications explicitly state that automatic recovery has stopped,
include the count/limit and channel/group scope, and request manual recovery.
They retain the existing root-admin recipient, notification preferences, rate
limit and delivery behavior. Email requires the configured email notification
path and working SMTP; this change does not force email or broadcast to all admins.
