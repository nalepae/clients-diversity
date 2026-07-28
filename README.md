# Ethereum clients distribution tracker

Tracks Ethereum **consensus-layer (CL)** and **execution-layer (EL)** client
diversity over time, measured by **block production**. It reads the
[client-identification](https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md)
codes that clients embed in beacon-block **graffiti** and aggregates them into
one point per UTC day.

<img width="1065" height="1323" alt="image" src="https://github.com/user-attachments/assets/6abf78ea-7268-4e5b-a119-3038bceebe0d" />
<img width="1025" height="575" alt="image" src="https://github.com/user-attachments/assets/183dff17-29c1-4923-b2d3-18fea9bffaba" />


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

- **EL:** `BU` Besu · `EG` Erigon · `EJ` EthereumJS · `EX` ethrex · `GE` Geth ·
  `NM` Nethermind · `RH` Reth
- **CL:** `CN` Caplin · `GR` Grandine · `LH` Lighthouse · `LS` Lodestar ·
  `NB` Nimbus · `PM` Prysm · `TK` Teku

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

Each option is a flag *or* an env var. The flag wins when both are set.

| Env / flag                       | Default            | Purpose                                         |
|----------------------------------|--------------------|-------------------------------------------------|
| `BEACON_URL` / `-beacon-url`     | _(required)_       | Beacon node REST base URL                       |
| `OUTPUT` / `-output`             | `../web/data.json` | Path to the JSON store                          |
| `REQ_TIMEOUT_SEC`                | `30`               | Per-request HTTP timeout (seconds)              |
| `MAX_RETRIES`                    | `3`                | Retries for transient beacon errors             |
| `GITHUB_TOKEN`                   | _(empty)_          | Token for the GitHub release lookups (optional) |

The tool is Ethereum **mainnet-only**.

The job is **incremental and resumable**: Re-running when already up to date is
a no-op, and a failed or interrupted run resumes from the last completed day.

### Output format (`data.json`)

```jsonc
{
  "meta": {
    "generatedAt": "2026-06-18T01:00:00Z",
    "startDate": "2026-05-22",
    "lastCompletedDate": "2026-06-17",
    "genesisTime": 1606824023,
    "secondsPerSlot": 12
  },
  "days": [
    {
      "date": "2026-05-22",
      "totalBlocks": 7123,
      "identifiedBlocks": 4210,
      "cl": { "PM": 1500, "LH": 900, ..., "unknown": 2913 },
      "el": { "GE": 2200, "NM": 800, ..., "unknown": 2913 },
      "clReleases": {
        "PM": { "5498": 1200, "54": 280, "": 20 },
        "LH": { "176c": 900 }
      },
      "elReleases": {
        "GE": { "117e": 2000, "9566": 200 },
        "NM": { "c07a": 800 }
      }
    }
  ],
  "releases": {
    "builds": {
      "GE": { "117e": "v1.17.3", "9566": "v1.16.9" },
      "PM": { "5498": "v7.1.5" }
    },
    "dates": {
      "GE": { "v1.17.3": "2026-05-11" },
      "PM": { "v7.1.5": "2026-06-17" }
    }
  }
}
```

### Test

```sh
cd fetcher && go test ./...
```

---

## `web/`: Visualize

A single static page (`web/index.html`).

```sh
python3 -m http.server 8000 --directory web
# open http://localhost:8000/
```

`data.json` must sit next to `index.html` in the `web/` directory.
