# neuPrintHTTP DSG auth integration

neuPrintHTTP delegates browser login, token issuance, identity, and per-dataset
authorization to DatasetGateway (DSG). The browser authN trio remains on DSG's
compatibility surface:

- `GET {dsg-url}/api/v1/authorize?redirect=<absolute-return-url>`
- `GET {dsg-url}/api/v1/long_lived_token`
- `GET {dsg-url}/api/v1/logout`

Per-dataset authorization uses DSG's native API under `/api/dsg/v1`.

## Configuration

```json
{
  "disable-auth": false,
  "dsg-url": "https://dsg.janelia.org",
  "dsg-cache-ttl": 300,
  "dsg-service-name": "neuprint",
  "hostname": "neuprint.janelia.org"
}
```

| Field | Required | Meaning |
|---|---|---|
| `dsg-url` | yes when auth is enabled | Base URL of the DatasetGateway service. |
| `dsg-cache-ttl` | no | Seconds to cache identity and non-TOS dataset decisions. Defaults to 300. The anonymous public-to-closed revocation window equals this TTL. |
| `dsg-service-name` | no | Service name sent to DSG native authorization. Defaults to `neuprint`. |
| `disable-auth` | no | Disables all authorization when true, for local/testing use only. Defaults to false and emits a prominent startup warning. |

Dataset vocabulary differences are registered in DSG with `DatasetAlias` rows.
The served neuPrint DB name is split at the first colon before authorization:
`hemibrain:v1.2.1` becomes native entry `{name: "hemibrain", version: "v1.2.1"}`.
Names without a colon omit `version`.

## Native identity

The optional authentication middleware runs on every `/api` request and checks
for a credential in this order:

1. `Authorization: Bearer <token>`
2. `dsg_token` cookie
3. `dsg_token` query parameter

Presence on any transport counts as an authentication attempt. Empty or
malformed Bearer headers, unsupported authorization schemes, empty cookie/query
values, and invalid or expired tokens return 401; they never downgrade to an
anonymous request. With no credential on any transport, the middleware leaves
the identity unset but installs the DSG client for anonymous decisions.

For an attempted valid credential it calls:

```http
GET {dsg-url}/api/dsg/v1/user
Authorization: Bearer <token>
```

Expected shape:

```json
{
  "id": 123,
  "email": "user@example.org",
  "name": "User Name",
  "picture_url": "https://example.org/avatar.png",
  "admin": false,
  "service_account": false
}
```

The middleware stores `dsg_identity`, `dsg_client`, `dsg_token`, and `email` in
the Echo context. `/profile` bypasses the identity cache to reflect current DSG
state.

## Native dataset decisions

Handlers call DSG for authenticated dataset decisions through:

```http
POST {dsg-url}/api/dsg/v1/authorize
Authorization: Bearer <token>
Content-Type: application/json
```

Request:

```json
{
  "service": "neuprint",
  "return_url": "https://neuprint.janelia.org/?dataset=hemibrain:v1.2.1",
  "entries": [
    {"name": "hemibrain", "version": "v1.2.1", "permission": "view"}
  ]
}
```

Response:

```json
{
  "entries": [
    {
      "name": "hemibrain",
      "version": "v1.2.1",
      "decision": "allow",
      "roles": ["view"]
    }
  ]
}
```

Anonymous reads use the same authorize endpoint and request shape but omit the
`Authorization` header entirely. They can receive only DSG's public `view`
decision. An anonymous `allow` proceeds, `tos_required` returns the same 403
JSON and opaque `tos_url`, and a denial returns 401 `authentication required`.
`service_eval` fails closed in neuPrintHTTP because this service uses linear
version evaluation; DAG ancestry remains the DVID client's responsibility.

Decision mapping:

| DSG decision | neuPrint behavior |
|---|---|
| `allow` + role `admin` | `ADMIN` |
| `allow` + role `manage` or `edit` | `READWRITE` |
| `allow` + role `view` or no roles | `READ` |
| `tos_required` | denied for the current request; response includes DSG's opaque `tos_url`. This decision is never cached. |
| `deny` | `NOAUTH` |
| `service_eval` | `NOAUTH` with a warning log; neuPrint expects DSG linear version evaluation. |

`RequireDatasetAccess` applies the global-admin shortcut before calling DSG, so
admins retain access to datasets not yet registered in DSG during rollout.
`/dataset-access` also applies that shortcut first, then force-refreshes the
queried dataset decision. This force-fresh call plus never caching
`tos_required` is the TOS-acceptance invalidation path.

Anonymous decisions have a per-dataset cache separate from every authenticated
token's cache. Both use `dsg-cache-ttl` (300 seconds by default), so a dataset
changed from public to closed may remain anonymously readable until that TTL
expires. `tos_required` is never cached in either cache.

Public access is always read-only. A user whose only role is public `view` is
denied mutations; mutation handlers require an explicit dataset `admin` grant.
DSG global administrators remain exempt through the global-admin shortcut.

Every `/api` route declares one of three policies: dataset guarded, admin, or a
named metadata-only exception. The current exceptions are `/api/serverinfo`,
`/api/vimoserver`, `/api/version`, `/api/available`, `/api/help/*`, and
`/api/dbmeta/datasets` (including its versioned alias). The dataset listing is
the one follow-up visibility exception; all data routes, including raw
key-value reads, authorize against their owning dataset.

With `disable-auth` enabled, the middleware installs a marked synthetic global
administrator (`disable-auth@localhost`). Every guard then takes the normal
admin shortcut, including reads, writes, raw key-value, and admin routes, while
making zero DSG calls.

For authenticated non-admin users, the dataset dropdown batches all served
dataset names into one native authorize call. It includes `allow` and
`tos_required` entries, excludes denies, skips the authorize call for admins,
and leaves hidden-dataset filtering unchanged. Anonymous listing visibility is
the named follow-up exception described above.

## Rollout checklist

1. In DSG admin, verify/create the `Service` row matching `dsg-service-name`
   with linear version evaluation.
2. Register each served neuPrint dataset and version in DSG.
3. Add `DatasetAlias` rows only where neuPrint's served name/version differs
   from DSG's canonical dataset/version names.
4. Remove any legacy name-translation config from the neuPrintHTTP deployment.
5. Rebuild the neuPrintHTTP container so the Go binary and neuPrintExplorer
   bundle ship together.
6. Validate on neuprint-test: public datasets appear for normal users, pending
   TOS redirects through `tos_url`, post-acceptance reload opens immediately,
   admins remain unrestricted, and `/login`, `/logout`, and `/token` still use
   the compatibility authN trio.
