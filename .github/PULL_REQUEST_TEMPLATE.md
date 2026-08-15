## What

<!-- What changes, in a sentence or two. -->

## Why

<!-- What breaks without this. Link the issue if there is one. -->

## Impact on consumers

<!-- Tightening a rule breaks builds that passed yesterday, even with no API
     change. Check the one that applies. -->

- [ ] No change to what goorg reports
- [ ] Fixes a false positive / loosens a rule — patch
- [ ] Adds a rule defaulting to `warning` — minor
- [ ] Adds a rule defaulting to `error`, or makes a rule stricter — **major**

## Checklist

- [ ] `task` passes (fmt, vet, test, dogfood)
- [ ] New or changed rules have a test proving they stay silent on conforming code
- [ ] `Doc` has a `Rationale:` and a `To fix:` section
- [ ] Rules table in `README.md` updated, if rules changed
