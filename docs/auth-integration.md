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
| `dsg-cache-ttl` | no | Seconds to cache identity and non-transient dataset decisions. Defaults to 300. |
| `dsg-service-name` | no | Service name sent to DSG native authorization. Defaults to `neuprint`. |
| `disable-auth` | no | Bypasses DSG auth when true, for local/testing use only. |

Dataset vocabulary differences are registered in DSG with `DatasetAlias` rows.
The served neuPrint DB name is split at the first colon before authorization:
`hemibrain:v1.2.1` becomes native entry `{name: "hemibrain", version: "v1.2.1"}`.
Names without a colon omit `version`.

## Native identity

Authentication middleware extracts a token in this order:

1. `Authorization: Bearer <token>`
2. `dsg_token` cookie
3. `dsg_token` query parameter

It calls:

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

Handlers call DSG for dataset decisions through:

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

The dataset dropdown batches all served dataset names into one native authorize
call. It includes `allow` and `tos_required` entries, excludes denies, skips the
authorize call for admins, and leaves hidden-dataset filtering unchanged.

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
