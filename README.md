# Ethereum clients distribution tracker

Tracks Ethereum **consensus-layer (CL)** and **execution-layer (EL)** client
diversity over time, measured by **block production**. It reads the
[client-identification](https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md)
codes that clients embed in beacon-block **graffiti** and aggregates them into
one point per UTC day.

<img width="1158" height="855" alt="image" src="https://github.com/user-attachments/assets/84ecc598-b41e-4957-95e3-7a10327d9b4e" />
<img width="1138" height="1282" alt="image" src="https://github.com/user-attachments/assets/c8f698d8-cb73-4e89-8a8e-baf6d87aebf2" />
<img width="1144" height="733" alt="image" src="https://github.com/user-attachments/assets/975f1d78-ee7d-4df9-96dc-802d9acc6da7" />
<img width="1148" height="855" alt="image" src="https://github.com/user-attachments/assets/dc5419f1-4263-4cc8-be59-34ca0a676f9b" />
<img width="1138" height="946" alt="image" src="https://github.com/user-attachments/assets/776ae73b-10a0-4134-b785-11675efc6548" />
<img width="1144" height="1256" alt="image" src="https://github.com/user-attachments/assets/e3c9b76f-e5aa-41a5-9c1f-5802dcc5eb29" />
<img width="1151" height="1290" alt="image" src="https://github.com/user-attachments/assets/2a97e5b5-33e1-434c-a5e6-6d879ba79b52" />
<img width="1153" height="1394" alt="image" src="https://github.com/user-attachments/assets/dd42d2ac-6655-41ad-a222-c10188e46f4f" />

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
