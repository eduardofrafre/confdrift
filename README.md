# confdrift

Staging works. Production doesn't. Somewhere between the two, a config key
went missing or a timeout lost a zero, and nobody noticed until the incident.

`confdrift` compares configuration files across environments and tells you
what drifted, before you deploy.

```console
$ confdrift k8s/staging/configmap.yaml k8s/production/configmap.yaml
DATABASE_URL  changed
  staging     "postgres://checkout:<redacted>@db.staging:5432/checkout"
  production  "postgres://checkout:<redacted>@db.prod:5432/checkout"

FEATURE_NEW_CART  missing from production
  staging     "true"
  production  (not set)

LOG_LEVEL  changed
  staging     "debug"
  production  "info"

PAYMENTS_TIMEOUT_MS  changed
  staging     "3000"
  production  "300"

STRIPE_API_KEY  changed
  staging     <redacted>
  production  <redacted>

5 of 6 keys drifted across 2 files (1 missing, 4 changed).
```

One static binary with no runtime and no config file. It exits non-zero on
drift, so it drops into CI as is.

## Install

Download a binary for Linux, macOS or Windows from
[Releases](https://github.com/eduardofrafre/confdrift/releases), or build it:

```sh
go install github.com/eduardofrafre/confdrift@latest
```

## Usage

```sh
confdrift .env.staging .env.production           # two environments
confdrift dev.yaml staging.yaml prod.yaml        # or any number of them
confdrift a.env b.env --keys-only                # only keys missing somewhere
confdrift a.env b.env --ignore 'DATABASE_*'      # skip keys expected to differ
confdrift a.env b.env --format json              # for scripts and bots
```

| Exit status | Meaning |
|---|---|
| 0 | The files agree |
| 1 | Drift found |
| 2 | Bad input: unreadable file, parse error, unknown flag |

### Formats

The format comes from the file name.

- **dotenv**: `.env`, `.env.production`, `prod.env`. Quotes, `export`, comments
  and multi-line values are understood. `${VARS}` are not expanded: if the
  reference itself changed, that is drift.
- **YAML** and **JSON**: nested keys flatten to paths such as `db.pool.max`,
  `servers[0]` or `labels["app.kubernetes.io/name"]`. YAML and JSON flatten the
  same way, so `app.yaml` can be compared with `app.json`.
- **Kubernetes ConfigMaps** compare by their `data` and `binaryData` keys. A
  file holding several manifests (`helm template`, `kustomize build` output)
  contributes the keys of every ConfigMap in it and skips the other resources.

Types count: `port: 5432` and `port: "5432"` are different values, because to
a Kubernetes manifest they are.

### Secrets

Values of keys that look like credentials (`*PASSWORD*`, `*TOKEN*`,
`*SECRET*`, `*API_KEY*` and similar) print as `<redacted>`, and so do passwords
inside URLs. They are still compared: a rotated secret still shows up as
drift. `--show-secrets` prints them.

## In CI

```yaml
# .github/workflows/config-drift.yml
on: pull_request
jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
      - run: go run github.com/eduardofrafre/confdrift@latest --keys-only deploy/staging.env deploy/production.env
```

`--keys-only` is usually what a pipeline wants: values are expected to differ
between environments, but a key defined in one and not the other is a bug
waiting for a deploy.

## License

MIT
