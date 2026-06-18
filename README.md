# Ethereum clients distribution tracker

Tracks Ethereum **consensus-layer (CL)** and **execution-layer (EL)** client
diversity over time, measured by **block production**. It reads the
[client-identification](https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md)
codes that clients embed in beacon-block **graffiti** and aggregates them into
one point per UTC day.

The repo is two independent pieces, one per directory:

| Directory   | What it is                                                                 |
|-------------|----------------------------------------------------------------------------|
| **`fetcher/`** | A Go daemon that backfills from a beacon node and writes `data.json`.   |
| **`web/`**     | A single static HTML page that reads `data.json` and draws the charts.  |

They communicate through one file, `data.json`, which the fetcher writes and
the page reads. Nothing else is shared.

---

## How client identification works

Clients append a 12-character segment to their graffiti, optionally after
arbitrary custom text. The segment interleaves the two layers:

```
[EL code:2][EL commit:4 hex][CL code:2][CL commit:4 hex]
 GE         117e             PM         5498              -> Geth + Prysm
```

The parser (`fetcher/internal/graffiti`) decodes the graffiti hex to text and
matches the trailing segment with the regex
`([A-Z]{2})([0-9a-f]{0,8})([A-Z]{2})([0-9a-f]{0,8})$`. A block counts as
**identified** only when *both* codes are present and valid against the
registries in `fetcher/internal/codes`:

- **EL:** `GE` Geth · `NM` Nethermind · `BU` Besu · `EG` Erigon · `RH` Reth ·
  `EJ` EthereumJS · `EX` ethrex · `TE` trin-execution
- **CL:** `PM` Prysm · `LH` Lighthouse · `TK` Teku · `NB` Nimbus ·
  `LS` Lodestar · `GR` Grandine

Anything that doesn't match a valid pair is bucketed as `unknown` for both
layers.

---

## `fetcher/`: Fetch & store

A self-contained Go module (`go 1.25`). The binary is a **daemon**: it runs the
ingestion job once on startup, then re-runs every day at **01:00 UTC**, forever.

### Run locally

```sh
cd fetcher
BEACON_URL=http://localhost:3500 go run .
```

### Configuration

Each option is a flag *or* an env var; the flag wins when both are set.

| Env / flag                       | Default            | Purpose                                  |
|----------------------------------|--------------------|------------------------------------------|
| `BEACON_URL` / `-beacon-url`     | _(required)_       | Beacon node REST base URL                |
| `OUTPUT` / `-output`             | `../web/data.json` | Path to the JSON store                   |
| `START_DATE` / `-start-date`     | `2026-05-22`       | Backfill start (UTC), used on first run  |
| `REQ_TIMEOUT_SEC`                | `30`               | Per-request HTTP timeout (seconds)       |
| `MAX_RETRIES`                    | `3`                | Retries for transient beacon errors      |

The tool is Ethereum **mainnet-only**.

The job is **incremental and resumable**: Re-running when already up to date is
a no-op, and a failed or interrupted run resumes from the last completed day.
Running at 01:00 UTC leaves the previous day's final slot ample time to finalize
(~13 min under normal conditions) before ingestion.

### Output format (`data.json`)

```jsonc
{
  "meta": {
    "generatedAt": "2026-06-18T01:00:00Z",
    "startDate": "2026-05-22",
    "lastCompletedDate": "2026-06-17",
    "genesisTime": 1606824023,
    "secondsPerSlot": 12,
    "clCodes": { "PM": "Prysm", "LH": "Lighthouse", ... },
    "elCodes": { "GE": "Geth", "NM": "Nethermind", ... }
  },
  "days": [
    {
      "date": "2026-05-22",
      "totalBlocks": 7123,
      "identifiedBlocks": 4210,
      "cl": { "PM": 1500, "LH": 900, ..., "unknown": 2913 },
      "el": { "GE": 2200, "NM": 800, ..., "unknown": 2913 }
    }
  ]
}
```

The `meta` block carries the code→name legends so the frontend needs no
hardcoded client list.

### Test

```sh
cd fetcher && go test ./...
```

---

## `web/`: Visualize

A single static page (`web/index.html`).
It fetches `./data.json` and draws two stacked-area charts (CL and EL), one
point per day.

```sh
python3 -m http.server 8000 --directory web
# open http://localhost:8000/
```

`data.json` must sit next to `index.html` in the `web/` directory.
