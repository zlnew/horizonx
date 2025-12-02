# go-monitor-agent

Lightweight Go daemon that scrapes local CPU, GPU, memory, disk, and system stats from `/proc` and `/sys` and exposes a JSON snapshot at `/metrics`.

## Requirements

- Go 1.25.4+.
- Linux host with `/proc` and `/sys` available.
- Optional tools/sensors:
  - `dmidecode` for DIMM specs (may need root).
  - `lspci` for GPU vendor/model detection.
  - Readable hwmon/RAPL/DRM entries for temps, power, and GPU busy percent; some platforms need extra capabilities.

## Run, build, test

- Run locally: `go run ./cmd/agent` (binds to `:3000`).
- Configure address: set `HTTP_ADDR` in the env or a `.env` file, e.g. `HTTP_ADDR=:8080`.
- Build: `go build -o bin/monitor-agent ./cmd/agent` or `make build`.
- Clean build artifact: `make clean`.
- Tests: `go test ./...` (none yet).

## Metrics

- Collectors refresh every second; `/metrics` always returns the latest snapshot.
- Each collector degrades gracefully—missing or unreadable inputs yield zeros rather than crashing the agent.
- Collected fields:
  - CPU: usage and per-core %, frequency (MHz), power (W) from RAPL/hwmon, temp (C), and spec from `/proc/cpuinfo`/cpufreq.
  - GPU: busy percent from `/sys/class/drm`, VRAM total/used (MB), temp/power from hwmon, and spec via `lspci`.
  - Memory: totals/available/used (GB), swap (GB), and DIMM specs from `dmidecode`.
  - Disk: total/free/used (GB) from the root filesystem and NVMe temp (C) when available.
  - System: hostname, OS pretty name, kernel version, uptime seconds.
  - Network: placeholder struct present in the payload but not yet populated.

### Endpoint

- `GET /metrics` returns JSON like:
  ```json
  {
    "cpu": {
      "spec": {
        "vendor": "GenuineIntel",
        "model": "11th Gen Intel(R) Core(TM) i7",
        "cores": 8,
        "threads": 16,
        "arch": "amd64",
        "base_freq": 1200,
        "max_freq": 4800
      },
      "usage": 12.5,
      "per_core": [8.2, 14.1, 13.3, 9.9],
      "watt": 7.8,
      "temp": 55.0,
      "frequency": 2200.0
    },
    "gpu": {
      "spec": {
        "vendor": "NVIDIA",
        "model": "NVIDIA Corporation GA104 [GeForce RTX 3070 Ti]"
      },
      "usage": 15.7,
      "vram_total": 8192,
      "vram_used": 2100,
      "temp": 48.3,
      "watt": 65.2
    },
    "memory": {
      "specs": [
        {
          "size": "16 GB",
          "type": "DDR4",
          "speed": "3200 MT/s",
          "manufacturer": "Micron",
          "part_number": "ABC123",
          "form_factor": "SODIMM"
        }
      ],
      "mem_total": 15.8,
      "mem_available": 9.4,
      "mem_used": 6.4,
      "swap_total": 2.0,
      "swap_free": 2.0,
      "swap_used": 0.0
    },
    "disk": {
      "total": 476.3,
      "free": 302.1,
      "used": 174.2,
      "temp": 41.5
    },
    "network": {
      "upload": 0,
      "download": 0
    },
    "system": {
      "hostname": "monitor-host",
      "os": "Ubuntu 22.04.4 LTS",
      "kernel": "6.5.0-45-generic",
      "uptime": 12345
    }
  }
  ```

## Project layout

- `cmd/agent`: Entrypoint wiring config/logger and starting the agent.
- `internal/agent`: Builds the registry, registers CPU/GPU/memory/disk/system collectors, and starts HTTP.
- `internal/core`: Metric types, registry, and 1s scheduler that refreshes snapshots.
- `internal/collector/*`: Source-specific collectors for CPU, GPU, memory, disk, and system metadata.
- `internal/transport/http`: Exposes `/metrics`.
- `internal/infra`: Config loader (`HTTP_ADDR`, `.env`) and logger interface/implementation.
- `pkg`: Small shared helpers.
- `bin/`: Build output; avoid committing binaries unless intentional.

## Notes

- The HTTP server runs until context cancellation; the scheduler stops when the agent shuts down.
