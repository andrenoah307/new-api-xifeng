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
channel per recovery pass, not per relay request. There is no frontend change.

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
