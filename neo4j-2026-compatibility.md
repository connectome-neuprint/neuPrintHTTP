# neuPrintHTTP compatibility with Neo4j 2026.08.1 (CalVer)

## Background

The `flyem-snapshot` ingestion pipeline is being upgraded from Neo4j 4.4.16 to
**2026.08.1**. (It originally targeted 5.26.27, was retargeted to the
calendar-versioned line at 2026.06.0, and then bumped through 2026.08.1 to
2026.08.1.)

Neo4j switched from semantic versioning to calendar versioning (`YYYY.MM.Patch`)
with the 2025.01 release — the first release after the 5.26 LTS checkpoint.

An earlier review concluded that neuPrintHTTP needed **no code changes** and only a
single deployment-config change. That conclusion holds for a *5.26* server. For a
CalVer server the config changes are still required, and the pinned Bolt driver sits
below the version Neo4j documents as forward-compatible with 2025.x/2026.x — which
is why this branch exists.

**Be clear about the strength of those two claims.** The config requirements are
established: `database` must be `data` or the service silently finds no data. The
driver bump is a *precaution taken from Neo4j's documented compatibility floor* — it
has **not** been shown that `v5.27.0` actually fails against a CalVer server. It may
negotiate a mutually supported Bolt version and work unchanged. See "Status" below;
that test is now runnable and is the thing to do before merging this.

---

## Status — what is NOT done

**Read this before assuming the change list below is sufficient.** The source change
in this branch is complete and compiles, but neuPrintHTTP is **not proven working
against a Neo4j 2026.08.1 server**, and **no running service has been changed.**

| # | Outstanding item | Owner / blocker |
|---|---|---|
| 1 | **The binary has not been run against a live Neo4j 2026.08.1 server.** Partly answered: the driver question *is* now measured — `v5.27.0` connects and queries a 2026.08.1 server successfully (see the `go.mod` section), so the bump is not required and is optional hygiene. What remains untested is neuPrintHTTP itself — its own queries, result handling, and the deployed config. | `flyem-snapshot`'s `neo4j-5-upgrade` branch has produced wasp, yakuba and fish2 databases validated on `2026.08.1`. Point the **current, unmodified** binary at one, bolt engine against `data`. |
| 2 | **The live deployment config has not been changed.** Nothing in this branch affects a running service; the files edited here are samples and examples. The `database` → `data` change must still be applied to the real config. | Blocked on identifying where the live config lives — see "changes required to config files installed elsewhere" |
| 3 | **Location of the live config is unconfirmed.** It is not tracked in this repo or in flyem-snapshot. Likely a Kubernetes ConfigMap or a mounted file. Until it is identified and written down here, this requirement exists only as prose. | Needs someone with deployment access |
| 4 | **Production Neo4j server has not been upgraded** to 2026.08.1. Required *before* a 2026.08.1-built database is swapped in. | Separate deployment, outside both repos |
| 5 | **Branch not pushed; no PR opened.** Committed locally only. | Awaiting review |
| 6 | **Driver `v6.x` not evaluated.** `v5.28.4` is documented only as *forward-compatible* with 2025.x/2026.x, whereas `v6.2.0` explicitly *supports* 4.4/5.x/2025.x/2026.x and is guaranteed forward-compatible with the next major release. Deliberately deferred as its own piece of work, since it is a major-version bump that may require source changes in `storage/neuprintbolt/`. | Deferred by choice, not blocked |

Suggested sequence: finish the flyem-snapshot Phase 2 ingestion run (which produces a
2026.08.1-built database), stand up a 2026.08.1 server against it, then use that to
close item 1. Items 2–4 are deployment work that must land together — see the
sequencing constraint below.

---

## Changes made in this repository

### `go.mod` / `go.sum` — Bolt driver bump (OPTIONAL; measured, see below)

```
github.com/neo4j/neo4j-go-driver/v5 v5.27.0  ->  v5.28.4
```

**This bump is not required for 2026.08.1. Measured, not assumed.** A minimal Go
program built against each driver version was pointed at a live `neo4j:2026.08.1`
server:

| driver | 2026.08.1 | 4.4.16 (control) |
|---|---|---|
| `v5.27.0` — what this repo ships today | **connects, `RETURN 1` → 1** | connects |
| `v5.28.4` — this bump | connects | connects |

Both negotiated the **bolt5** protocol. The legacy fixed handshake was therefore
sufficient: 2026.08.1 still accepts a Bolt version `5.27` can offer, and
Handshake Manifest v1 never came into play.

So keep this bump only as forward-looking hygiene — sitting on the documented
floor so that a *future* server which drops older Bolt versions does not break
us. It is not a fix, and it is separable: the config changes below are the part
that is actually required, and they are in their own commit.

What the measurement does **not** establish: it exercises a trivial query
through the driver alone, not the queries neuPrintHTTP issues, its result
handling, or the deployed configuration. Only the full-binary test in Status
closes that.

Neo4j documents driver **5.28** as the forward-compatibility floor for 2025.x and
2026.x servers; **5.27 is not covered**. That is a documentation boundary, and
the measurement above shows it is not yet a functional one — nothing
demonstrates that `5.27` cannot talk to a
`2026.08.1` server.

The mechanism matters here, because it is not merely a documentation boundary.
Driver `v5.28.0` introduced **Bolt Handshake Manifest v1** (ADR 30), which is how a
driver discovers and negotiates the full range of Bolt protocol versions a newer
server offers. `v5.27.0` still performs the legacy fixed handshake.

`v5.28.4` was chosen rather than `v6.x` deliberately:

- It is a **drop-in** upgrade. The driver README states that upgrades within a major
  version never contain breaking API changes.
- `v6.2.0` explicitly supports 4.4.x / 5.x / 2025.x / 2026.x and is guaranteed
  forward-compatible with the next major release, but it is a major-version bump that
  may require source changes in `storage/neuprintbolt/`. That is worth doing as its
  own piece of work, not smuggled into a compatibility fix.

Verified locally with Go 1.26.5: `go build ./...` and `go vet ./storage/...` both
pass cleanly (exit 0). The change touches one line of `go.mod` and adds two lines to
`go.sum` — no transitive dependency churn.

### `bolt-config.json` — database name

```
"database": "neo4j"  ->  "database": "data"
```

The flyem-snapshot pipeline builds its database as `data` (it runs
`neo4j-admin database import full data`), not the Neo4j default of `neo4j`.

This is a **silent** failure mode if wrong: neuPrintHTTP connects successfully and
then finds none of the expected data.

### `config-examples/config.json` — dead engine (pre-existing bug)

The canonical example config specified `"engine": "neuPrint-neo4j"`. That backend
speaks the **legacy HTTP transactional API, which was removed in Neo4j 5** — so the
primary example in this repo has been pointing users at a backend that cannot work
against any Neo4j 5.x or CalVer server.

Updated (both the main store and the `mainstore-alternatives` entry) to:

- `"engine": "neuPrint-bolt"`
- `"server": "bolt://<NEO4J-SERVER>:7687"` (was the HTTP port `7474`)
- added `"database": "data"`

This bug predates the CalVer retarget; it was fixed here because it would otherwise
guarantee a broken deployment for anyone following the examples.

### `README.md`

- Corrected the Bolt example's `database` value and replaced the stale comment
  (`// Optional: database name for Neo4j 4.0+ (omit for Neo4j 3.x)`) with a note
  explaining the `data` name and its silent failure mode.
- Stated explicitly that `neuPrint-neo4j` is not merely "deprecated" but
  **non-functional** against Neo4j 5.x and later.

---

## IMPORTANT — changes required to config files installed elsewhere

**The files changed in this repository are samples and examples. Changing them does
not change any running service.** The live configuration is deployed outside this
repository and must be edited separately.

### 1. The live neuPrintHTTP config — `database` must become `data`

Wherever the running service reads its config, the main store's `engine-config`
must be:

```json
"engine-config": {
    "server": "bolt://<neo4j-host>:7687",
    "database": "data"
}
```

Two things to confirm at the same time in that same live config:

- **`"engine"` must be `"neuPrint-bolt"`.** If any deployment is still on
  `"neuPrint-neo4j"`, it is already broken against Neo4j 5+ and will remain broken.
- **Every** store entry needs the correct database name, not just the primary one —
  including any `mainstore-alternatives` entries and any per-dataset stores.

> **Location not yet confirmed.** This config is not tracked in either this repo or
> flyem-snapshot. Based on `shell_scripts/restart_instance.sh` in flyem-snapshot
> (which scales `neo4j` and `neuprinthttp` replicas), the deployment appears to be
> Kubernetes, so the live value is most likely in a **ConfigMap** or a file mounted
> into the pod — not in git. **This needs to be identified and recorded**, otherwise
> the requirement survives only as prose in a design document.

### 2. The Neo4j server itself must be upgraded to 2026.08.1 — in lockstep

This is a sequencing constraint, not an optional step.

A store built by the 2026.08.1 importer **cannot be read by an older server**, and
Neo4j provides no downgrade path. The production server (under
`/data1/data15/app/neo4j/data/db/` per flyem-snapshot's `restart_instance.sh`) is a
separate deployment from the ingestion container, and its own `neo4j.conf` is *not*
the one in the flyem-snapshot repo.

Required order:

1. Upgrade the production Neo4j server to 2026.08.1
2. Deploy neuPrintHTTP built from this branch (driver ≥ 5.28.4)
3. Update the live config's `database` to `data`
4. *Then* swap in the 2026.08.1-built database

Doing step 4 before steps 1–3 leaves a database no running server can open.

### 3. Server-side config notes (for whoever owns the production `neo4j.conf`)

These apply to the production server's own config, which is outside both repos.
For reference, the equivalent changes made to the flyem-snapshot *ingestion*
`neo4j.conf` were:

- **`db.tx_state.memory_allocation` was removed** (without replacement) in 2025.01 —
  delete it.
- **`server.memory.off_heap.*` was removed** in 2025.01.
- **`-XX:-UseBiasedLocking` must be removed** from `server.jvm.additional`. Neo4j
  2025.01+ requires **Java 21**, where biased locking no longer exists; the flag is
  at best ignored with an obsolete-option warning and at worst rejected as an
  unrecognized VM option, preventing JVM startup.
- **Pin `db.query.default_language=CYPHER_5`.** As of 2025.06 Cypher is versioned
  separately from the server, and from 2026.02 the *distributed* `neo4j.conf` sets
  `CYPHER_25`. Pinning avoids the language version shifting on a future server bump.
- Beware `server.config.strict_validation.enabled=false` — with it set, removed or
  misspelled settings are **silently ignored** rather than failing startup, which is
  exactly how stale config goes unnoticed.

---

## What was checked and found to be fine

- **No server-version parsing anywhere in neuPrintHTTP.** This was checked
  specifically because `2026.08.1` contains a leading zero and would fail strict
  semver parsing. The `VERSION = "0.5.0"` constant in
  `storage/neuprintbolt/neuprintbolt.go` is neuPrintHTTP's *own* engine version, not
  the server's. The only call touching server identity is
  `driver.VerifyConnectivity(ctx)`. There is no `dbms.components()` query and no
  version gate.
- **Cypher queries are safe.** The queries issued are plain `MATCH`/`RETURN` (e.g.
  the `:Meta` lookup in `neuprintbolt.go`) and are valid under Cypher 5, which the
  ingestion config pins server-side.
- **neuprint-python and neuPrintExplorer need no changes.** Both talk only HTTP to
  neuPrintHTTP and have no direct Neo4j connection.
- **No hardcoded default database name anywhere in the Go source.** `dbName` is read
  purely from config (`storage/neuprintbolt/neuprintbolt.go`), and a repo-wide search
  for `"neo4j"` as a database literal in `*.go` returns nothing. So the config files
  corrected in this branch were genuinely the only places carrying the wrong name —
  there is no compiled-in fallback that also needs changing.

  **However:** if the `database` field is *omitted* from the config, the driver falls
  back to the server's **home database**, which on the production server will be
  whatever its own `initial.dbms.default_database` is set to (most likely `neo4j`).
  Omitting the field therefore fails in exactly the same silent way as setting it
  wrong. It must be present and explicit.

---

## Verification performed

| Check | Result |
|---|---|
| `go build ./...` (Go 1.26.5) | pass (exit 0) |
| `go vet ./storage/...` | pass (exit 0) |
| `bolt-config.json` parses as JSON | valid |
| `config-examples/config.json` parses as JSON | valid |
| Driver upgrade diff scope | 1 line in `go.mod`, 2 added in `go.sum`; no transitive churn |

**Not yet verified:** no runtime test against an actual Neo4j 2026.08.1 server. That
is the remaining gap and should be done before production deployment — ideally
against the wasp v0.8 database produced by the upgraded flyem-snapshot pipeline.

Everything still outstanding — including this — is tracked in
[Status — what is NOT done](#status--what-is-not-done) at the top of this document.
Nothing in the table below should be read as "ready to deploy."

## Rollback

Revert this branch. The driver bump is the only code change to the service;
the config and README edits affect samples and documentation only. Note that
rolling back neuPrintHTTP does **not** by itself restore a working system if the
production server and database have already been moved to 2026.08.1.
