# SOX Monitor

A controlled experiment in **Go + PostgreSQL** that simulates bank transaction
requests between a bank and its account holders — individuals (*Pessoa Física*,
PF) and companies (*Pessoa Jurídica*, PJ) — monitors every transaction, and
flags the ones that fall outside an expected pattern using a set of
**SOX-inspired internal controls**.

It is intentionally small and readable: the goal is to demonstrate, end to end,
how an out-of-pattern detection pipeline and an auditable control environment
fit together — not to ship a production payment system.

---

## What is SOX (in a banking context)?

The **Sarbanes–Oxley Act of 2002 (SOX)** is a U.S. federal law enacted after
the Enron and WorldCom accounting scandals. It sets requirements for the
accuracy and integrity of financial reporting in public companies.

For a bank — where the "financial records" are the transactions themselves —
two sections are especially relevant:

- **Section 404 — Internal Controls.** The institution must maintain and
  continuously evaluate internal controls over financial reporting. In
  practice this means automated, testable controls that detect anomalous or
  unauthorized activity.
- **Section 802 — Records & Penalties.** Altering, destroying, or falsifying
  records is a criminal offense. This translates directly into the need for an
  **immutable, tamper-evident audit trail**.

Two further principles that SOX programs rely on and that this project models:

- **Segregation of Duties (SoD):** the person who initiates a sensitive
  operation must not be the same person who approves it.
- **Traceability:** every control threshold and every schema change must be
  documented, versioned, and auditable.

This project does not claim SOX *certification* — it is a teaching model of how
these principles are implemented in code and in the database.

---

## How the principles map to the code

| SOX principle | Where it lives in this project |
|---|---|
| §404 internal controls | Continuous detection engine in `internal/monitor` |
| §802 immutable records | `auditoria` table written by DB triggers; UPDATE/DELETE blocked |
| Segregation of Duties | `iniciado_por` / `aprovado_por` fields + detection rule R5 |
| Traceability | Versioned migrations (`schema_migrations`) + env-tunable thresholds |

---

## Detection rules

The engine evaluates each transaction against six rules. Default thresholds
live in `monitor.ConfigPadrao()` and can be overridden via `SOX_*` environment
variables.

| Rule | Detects | Default threshold | Severity |
|---|---|---|---|
| **R1** Amount ceiling | Value above an absolute cap | R$ 50,000 (2× → critical) | High / Critical |
| **R2** Statistical deviation | z-score vs. the account's own history | ≥ 3 std devs (needs ≥ 5 samples) | Medium → Critical |
| **R3** Atypical timing | Outside 06:00–22:00 or on weekends | — (PJ weighted higher than PF) | Low / Medium |
| **R4** Abnormal velocity | Too many transactions in a window | > 5 in 1 hour | High |
| **R5** Segregation of Duties | High-value transaction without an independent approver | ≥ R$ 10,000 requires approver ≠ initiator | High |
| **R6** Structuring (smurfing) | Repeated values just under the regulatory limit | ≥ 3 in the [90%, 100%) band | Critical |

A transaction with zero alerts is recorded as `APROVADA`; any transaction that
triggers one or more rules is recorded as `EM_ANALISE`.

---

## Project structure

```
banco-sox/
├── cmd/
│   ├── migrate/main.go        # migration runner (up / down / version)
│   └── simulador/main.go      # bootstrap + scenario simulation
├── internal/
│   ├── model/model.go         # domain structs
│   ├── monitor/               # SOX rule engine (zero external deps)
│   │   ├── monitor.go
│   │   └── monitor_test.go
│   ├── config/config.go       # configuration via environment variables
│   ├── db/db.go               # PostgreSQL access (database/sql + lib/pq)
│   └── audit/audit.go         # read-only access to the audit trail
├── migrations/
│   ├── 0001_schema.up.sql / .down.sql
│   └── 0002_audit.up.sql  / .down.sql
├── docker-compose.yml         # PostgreSQL (+ optional app services)
├── Dockerfile                 # multi-stage static build of both binaries
├── Makefile                   # orchestration entry point
└── .env.example               # connection + SOX_* thresholds
```

**Design note.** The `internal/monitor` package has **no external dependencies**
and never touches the database — it operates only on plain structs. The `db`
layer fetches and persists; the `monitor` layer judges. This separation is what
makes the controls unit-testable in isolation.

Money is always stored as **centavos (`int64`)**, never as floating point, to
avoid rounding errors in financial values.

---

## How to run

### Prerequisites

- Go (1.22+)
- Docker (with Docker Compose)

### Quick start

The happy path is a single command that brings up the database, applies
migrations, seeds baseline data, and runs the simulation:

```bash
make tudo
```

### Step by step

```bash
make up           # start ONLY the PostgreSQL container
make migrate-up   # apply migrations 0001 and 0002
make bootstrap    # create bank / PF / PJ / accounts + baseline history
make simular      # run the scenarios through the monitoring engine
```

### Run everything inside Docker (no Go on the host)

```bash
make docker-tudo
# equivalent to: docker compose up --build --abort-on-container-exit
```

### Useful commands

| Command | Purpose |
|---|---|
| `make version` | List applied migrations and when |
| `make test` | Run the rule-engine unit tests (no database needed) |
| `make psql` | Open `psql` inside the container |
| `make reset` | Tear everything down and wipe the volume |

### Configuration

Connection settings and SOX thresholds are read from the environment (see
`.env.example`):

```
PGHOST=localhost
PGPORT=5432
PGUSER=banco
PGPASSWORD=banco
PGDATABASE=banco_sox
PGSSLMODE=disable

# Optional control thresholds (values in centavos)
# SOX_TETO_CENTAVOS=5000000
# SOX_ZSCORE=3.0
# SOX_SOD_TETO_CENTAVOS=1000000
# SOX_LIMITE_REGULATORIO_CENTAVOS=1000000
# SOX_JANELA_VELOCIDADE=1h
```

---

## Why testing matters here

In a SOX context a control is only credible if it can be **demonstrated and
re-verified on demand**. This project is built so that the controls are
testable at three levels:

1. **Unit tests of the rules.** Because `internal/monitor` is pure Go with no
   I/O, every rule is covered by deterministic tests (`monitor_test.go`) that
   run in milliseconds without a database. They include both positive cases
   (the rule must fire) and negative cases (e.g. R2 must *not* fire without
   enough history; R5 must *not* fire when an independent approver exists).
   These tests act as the **executable specification** of each control.

   ```bash
   make test
   ```

2. **End-to-end simulation.** `cmd/simulador` seeds a believable "normal"
   baseline and then plants one anomaly per rule, persisting transactions and
   alerts to PostgreSQL. The final report aggregates alerts by rule and prints
   the audit-trail totals, so you can see the whole pipeline behave on real
   data.

3. **Audit-trail immutability.** The §802 control can be verified directly in
   the database. Open `make psql` and try to tamper with the trail:

   ```sql
   UPDATE auditoria SET operacao = 'x' WHERE id = 1;
   -- ERROR: Trilha de auditoria imutavel (SOX 802): operacao UPDATE proibida.

   DELETE FROM auditoria WHERE id = 1;
   -- ERROR: Trilha de auditoria imutavel (SOX 802): operacao DELETE proibida.
   ```

   Both are rejected by a trigger; the trail is append-only.

A representative run produces a summary like:

```
== alerts by rule ==
  R1  Amount ceiling              HIGH      1
  R2  Statistical deviation       CRITICAL  6
  R3  Atypical timing             MEDIUM    1
  R4  Abnormal velocity           HIGH      5
  R5  Segregation of Duties       HIGH      1
  R6  Structuring                 CRITICAL  2

== audit trail (SOX) ==
  immutable records: 137
```

Note that a single planted scenario can legitimately trigger **more than one**
rule (for example, a structuring amount that is also far above the account's
historical average will fire R6 *and* R2). The engine accumulates all
applicable alerts rather than stopping at the first match.

---

## Scope and honest limitations

- This is a **batch-style didactic simulator**, not a service: no HTTP API, no
  concurrency, no asynchronous re-evaluation.
- Audit immutability is enforced by a trigger. Against a database superuser who
  can `DROP TRIGGER` it is not absolute; in production it would be reinforced
  with restricted privileges and/or an external append-only sink.
- Thresholds and scenarios are illustrative, not calibrated against real
  regulatory limits.

Natural next steps would be to expose the evaluation behind an endpoint, move
detection into an asynchronous worker, and add integration tests that run
against the containerized database.