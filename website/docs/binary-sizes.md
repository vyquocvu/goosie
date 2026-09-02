# Binary Size Tracking

| Date | Commit | Go Version | OS/Arch | Size (bytes) | Change |
|---|---|---|---|---|---|
| 2026-07-14 | 0261c7f | go1.24.9 | linux/amd64 | 43636288 | Baseline |
| 2026-09-02 | 0d7858d | go1.26.5 | darwin/arm64 | 33884322 | -9.75 MB (-22.3%) |

## Methodology & Verification

- Binary sizes are measured with stripped symbols (`-ldflags="-s -w"`).
- CI gate is enforced by `cmd/browser/binary_size_test.go` with a hard limit of 100 MB (`maxBinarySize`).
- Significant size reductions were achieved through `/internal` dead code elimination, compact DOM structures, and property atom deduplication.
