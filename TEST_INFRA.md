# TEST_INFRA: Goosie Browser Performance & Memory Test Infrastructure

## 1. Test Philosophy & Architecture

The Goosie browser performance testing infrastructure provides **opaque-box, metric-driven, multi-tier verification** of browser engine responsiveness, memory containment, CPU conservation, and concurrency safety.

The testing infrastructure is structured into a **4-Tier Performance Verification Matrix**:

| Tier | Category | Objectives | Test Target |
|---|---|---|---|
| **Tier 1** | **Feature Metric Verification** | Direct validation of core resource budgets and limits (CPU, RSS/Heap memory, scroll latency, cache boundaries, goroutine counts). | Core subsystems (`ui`, `renderer`, `image`, `memory`, `js`) |
| **Tier 2** | **Boundary & Stress Conditions** | Extreme stress testing: rapid scroll gestures, 4K high-res image scaling, deep DOM trees (>3000 commands), zero-delay timers. | Algorithmic boundaries & edge cases |
| **Tier 3** | **Cross-Feature Integration** | Concurrency and lifecycle safety: navigating during pending downloads, scrolling during background decodes, multi-tab resource reclamation. | Subsystem interoperability & race safety |
| **Tier 4** | **Real-World Workload Simulation** | End-to-end simulation of real production websites (`https://peach.blender.org/` offline mirror and live) and standardized long pages. | User-perceptible responsiveness & stability |

---

## 2. Test Suite Catalog & Layout

All performance test suites and benchmarks are organized under `test/e2e/`:

```
test/e2e/
├── perf_benchmark_test.go      # 4-Tier performance & memory test suite + Go benchmarks
├── scroll_perf_test.go         # Scroll FPS live logging & pacing test
├── peach_blender_test.go       # Online live E2E rendering test for Peach Blender
├── testdata/
│   ├── peach_blender/
│   │   └── index.html          # Authentic 25KB offline Peach Blender mirror fixture
│   └── results/                # Visual comparison and screenshot output directory
└── utils.go                    # Image comparison and diffing helpers
```

---

## 3. Performance Budgets & Acceptance Thresholds

| Metric | Target Budget | Hard Failure Limit | Authoritative Source |
|---|---|---|---|
| **Idle CPU Utilization** | `~0.0% – 1.0%` | `< 5.0%` | `ORIGINAL_REQUEST.md` line 174 |
| **Process Memory Footprint** | `< 250 MB` | `< 300.0 MB` (down from ~868 MB) | `ORIGINAL_REQUEST.md` line 176 |
| **Average Scroll Frame Latency**| `< 2.0 ms` | `< 16.0 ms` (> 60 FPS) | `ORIGINAL_REQUEST.md` line 171 |
| **Image Cache Memory Limit** | `32 MB – 64 MB` | `<= 64 MB` with LRU eviction | `PROJECT.md` Feature 5 |
| **Goroutine Lifecycle Leak** | `0 delta` | `<= 2` runtime variance | `DISPATCH.md` line 33 |
| **HTML5 Timer Clamping** | `>= 4.0 ms` | `>= 35 ms` per 10 zero-delay ticks | HTML5 Spec §8.5.2 |
| **High-Res Downscaling** | `> 95%` reduction | Clamped to layout dimensions $\times$ DPR | `PROJECT.md` Feature 4 |

---

## 4. Test Execution Commands

### 4.1 Master 4-Tier Test Suite
Run the entire 14-test performance suite:
```bash
go test -v ./test/e2e -run TestPerf
```

### 4.2 Tier-Specific Runs
```bash
# Tier 1: Feature Metric Verification (Idle CPU, Memory, Latency, Caches, Goroutines)
go test -v ./test/e2e -run TestPerf_Tier1

# Tier 2: Boundary & Stress Conditions (Rapid Scroll, 4K Downscale, Deep DOM, Timer Clamping)
go test -v ./test/e2e -run TestPerf_Tier2

# Tier 3: Cross-Feature Integration (Pending Navigation, Concurrent Scroll/Decode, Multi-Tab)
go test -v ./test/e2e -run TestPerf_Tier3

# Tier 4: Real-World Workloads (Peach Blender Scorecard, Long Page Benchmark)
go test -v ./test/e2e -run TestPerf_Tier4
```

### 4.3 Go Benchmark Suite
Run the high-throughput Go performance benchmarks:
```bash
go test -bench=Benchmark -benchtime=500ms ./test/e2e -run=^$
```

### 4.4 Real-World Peach Blender Live Conformance (Online)
```bash
go test -tags="e2e,online" -v ./test/e2e -run TestPeachBlenderRendering
```

---

## 5. Profiling & Diagnostic Harnesses

### 5.1 CPU Profiling
Generate pprof CPU profiles during scroll or page load:
```bash
go test -cpuprofile=cpu.pprof ./test/e2e -run TestPerf_Tier4_PeachBlenderWorkload
go tool pprof -top -cum cpu.pprof
```

### 5.2 Heap Allocation Profiling
Capture heap memory allocation profiles to diagnose retained buffers and texture allocations:
```bash
go test -memprofile=mem.pprof ./test/e2e -run TestPerf_Tier1_MemoryFootprint
go tool pprof -top -cum mem.pprof
```

### 5.3 Goroutine Leak Inspection
Inspect active stack traces to pinpoint unclosed goroutines:
```bash
go test -v ./test/e2e -run TestPerf_Tier1_GoroutineLifecycleLeaks
```

---

## 6. Continuous Integration & Regression Gates

1. **Pre-Commit / Pre-Push Gate**:
   - `go test -v ./test/e2e -run TestPerf_Tier1` must execute and report metrics.
   - `BenchmarkScrollLatency_LongPage` must maintain < 16ms/op.
2. **Visual Fidelity Gate**:
   - Visual regression checks via Playwright (`CompareGoosieVsBrowser`) must ensure performance fixes do not alter visual layout or exceed established diff thresholds.
