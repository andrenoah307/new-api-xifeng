#!/usr/bin/env node
// 3-way semantic JSON merge for i18n locale files during upstream merges.
// Usage: node scripts/merge-i18n-3way.js <base.json> <ours.json> <theirs.json> <out.json>
// Strategy: start from ours (preserves local key order and customizations);
// apply upstream additions/changes/deletions unless ours also changed the key
// (ours wins on conflict).
const fs = require('fs')

const [base, ours, theirs, out] = process.argv.slice(2).map((p, i) =>
  i < 3 ? JSON.parse(fs.readFileSync(p, 'utf8')) : p
)

const isObj = (v) => v !== null && typeof v === 'object' && !Array.isArray(v)

function merge3(b, o, t) {
  const result = {}
  for (const k of Object.keys(o)) result[k] = o[k]

  for (const k of Object.keys(t)) {
    const inBase = isObj(b) && Object.prototype.hasOwnProperty.call(b, k)
    const inOurs = Object.prototype.hasOwnProperty.call(o, k)
    if (!inBase) {
      // added by theirs
      if (!inOurs) result[k] = t[k]
      else if (isObj(o[k]) && isObj(t[k])) result[k] = merge3({}, o[k], t[k])
      // both added scalar: ours wins
    } else {
      const bv = JSON.stringify(b[k])
      if (JSON.stringify(t[k]) === bv) continue // theirs unchanged
      if (!inOurs) continue // ours deleted, theirs changed: ours wins
      if (isObj(o[k]) && isObj(t[k])) {
        result[k] = merge3(isObj(b[k]) ? b[k] : {}, o[k], t[k])
      } else if (JSON.stringify(o[k]) === bv) {
        result[k] = t[k] // ours unchanged, take theirs
      } // else both changed: ours wins
    }
  }

  if (isObj(b)) {
    for (const k of Object.keys(b)) {
      // deleted by theirs, unmodified by ours -> delete
      if (
        !Object.prototype.hasOwnProperty.call(t, k) &&
        Object.prototype.hasOwnProperty.call(result, k) &&
        JSON.stringify(o[k]) === JSON.stringify(b[k])
      ) {
        delete result[k]
      }
    }
  }
  return result
}

fs.writeFileSync(out, JSON.stringify(merge3(base, ours, theirs), null, 2) + '\n')
