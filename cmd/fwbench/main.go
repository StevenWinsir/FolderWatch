// fwbench measures FolderWatch using generated, disposable fixtures only.
// It is a developer tool, not part of the shipped folderwatch executable.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/testutil"
)

type report struct {
	GoVersion                 string      `json:"go_version"`
	OS                        string      `json:"os"`
	Arch                      string      `json:"arch"`
	CPUs                      int         `json:"logical_cpus"`
	Files                     int         `json:"files"`
	BytesPerFile              int         `json:"bytes_per_file"`
	Depth                     int         `json:"depth"`
	Binary                    bool        `json:"binary"`
	Samples                   int         `json:"samples"`
	Burst                     int         `json:"burst"`
	DebounceMS                float64     `json:"debounce_ms"`
	ReadyMS                   float64     `json:"prepare_and_ready_ms"`
	SettledMS                 float64     `json:"prepare_and_settled_ms"`
	HeapBytes                 uint64      `json:"settled_heap_bytes"`
	HeapSysBytes              uint64      `json:"settled_heap_sys_bytes"`
	AllocatedBytes            uint64      `json:"startup_allocated_bytes"`
	PeakRSSBytes              uint64      `json:"peak_rss_bytes"`
	RetainedMemoryBytes       int64       `json:"retained_content_bytes"`
	RetainedDiskBytes         int64       `json:"retained_disk_bytes"`
	IdleSeconds               float64     `json:"idle_seconds"`
	IdleCPUPercent            float64     `json:"idle_cpu_percent_one_core"`
	IdleEvents                int         `json:"idle_events"`
	LatencyMS                 []float64   `json:"write_complete_to_core_state_ms"`
	P95MS                     float64     `json:"p95_write_complete_to_core_state_ms"`
	AfterNominalDebounceP95MS float64     `json:"p95_minus_nominal_debounce_ms"`
	DiffStatus                diff.Status `json:"diff_status"`
	DiffMS                    float64     `json:"diff_ms"`
	Cycles                    int         `json:"additional_start_stop_cycles"`
	GoroutinesBefore          int         `json:"goroutines_before"`
	GoroutinesActive          int         `json:"goroutines_active"`
	GoroutinesAfter           int         `json:"goroutines_after"`
	FDsBefore                 int         `json:"fds_before"`
	FDsActive                 int         `json:"fds_active"`
	FDsAfter                  int         `json:"fds_after"`
	CacheEntriesAfter         int         `json:"cache_entries_after"`
	Passed                    bool        `json:"correctness_and_cleanup_passed"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fwbench:", err)
		os.Exit(1)
	}
}

func run() error {
	files := flag.Int("files", 1000, "generated file count, 1..50000")
	size := flag.Int("bytes", 1024, "bytes per generated file, 16..134217728")
	depth := flag.Int("depth", 1, "generated directory depth, 1..64")
	binary := flag.Bool("binary", false, "generate NUL-containing data")
	samples := flag.Int("samples", 20, "verified write bursts, 0..1000")
	burst := flag.Int("burst", 100, "files changed per burst")
	idle := flag.Duration("idle", 10*time.Second, "idle CPU observation window")
	cycles := flag.Int("cycles", 0, "additional full start/stop cycles")
	cpuPath := flag.String("cpu-profile", "", "optional CPU profile output")
	heapPath := flag.String("heap-profile", "", "optional settled heap profile output")
	goroutinePath := flag.String("goroutine-profile", "", "optional active goroutine profile output")
	flag.Parse()
	if flag.NArg() != 0 || *files < 1 || *files > 50000 || *size < 16 || *size > 128<<20 || *depth < 1 || *depth > 64 || *samples < 0 || *samples > 1000 || *burst < 1 || *burst > *files || *idle < 0 || *idle > time.Hour || *cycles < 0 || *cycles > 1000 {
		return errors.New("invalid workload bounds (burst must not exceed files)")
	}
	if int64(*files)*int64(*size) > 2<<30 {
		return errors.New("generated fixture exceeds 2GiB safety budget")
	}
	parent, err := os.MkdirTemp("", "folderwatch-bench-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(parent)
	root, cache := filepath.Join(parent, "fixture 项目"), filepath.Join(parent, "cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		return err
	}
	// Snapshot caches are confined to this disposable parent, separate from root.
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if err := os.Setenv(key, cache); err != nil {
			return err
		}
	}
	dir := root
	for i := 1; i < *depth; i++ {
		dir = filepath.Join(dir, "deep")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data := []byte(strings.Repeat("x", *size))
	data[len(data)-1] = '\n'
	if *binary {
		data[8] = 0
	}
	paths := make([]string, *files)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("file-%05d.txt", i))
		if err := os.WriteFile(paths[i], data, 0600); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	r := report{GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Files: *files, BytesPerFile: *size, Depth: *depth, Binary: *binary, Samples: *samples, Burst: *burst, Cycles: *cycles}
	runtime.GC()
	r.GoroutinesBefore, r.FDsBefore = runtime.NumGoroutine(), countFDs()
	var initial runtime.MemStats
	runtime.ReadMemStats(&initial)
	stopProfile, err := cpuProfile(*cpuPath)
	if err != nil {
		return err
	}
	defer stopProfile()
	started := time.Now()
	prepared, err := app.Prepare(ctx, root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		return err
	}
	r.DebounceMS = float64(prepared.Config.Debounce) / float64(time.Millisecond)
	s, err := app.StartSession(ctx, prepared)
	if err != nil {
		return fmt.Errorf("start session: %w (macOS kqueue needs a descriptor per file; check ulimit -n)", err)
	}
	defer s.Close()
	r.ReadyMS = elapsedMS(started)
	// An explicit owner barrier closes startup reconciliation before observation.
	if err := s.Pause(ctx); err != nil {
		return err
	}
	if err := s.Resume(ctx); err != nil {
		return err
	}
	r.SettledMS = elapsedMS(started)
	prepared.Inventory.Entries = nil
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	r.HeapBytes, r.HeapSysBytes, r.AllocatedBytes = mem.HeapAlloc, mem.HeapSys, mem.TotalAlloc-initial.TotalAlloc
	r.GoroutinesActive, r.FDsActive = runtime.NumGoroutine(), countFDs()
	stats := s.SnapshotStats()
	r.RetainedMemoryBytes, r.RetainedDiskBytes = stats.MemoryBytes, stats.DiskBytes
	if err := writeProfile(*heapPath, "heap", 0); err != nil {
		return err
	}
	if err := writeProfile(*goroutinePath, "goroutine", 2); err != nil {
		return err
	}
	// Drain startup publications; measure idle after the owner barrier, not sleep alone.
	drainEvents(s)
	cpuBefore, _, err := processUsage()
	if err != nil {
		return err
	}
	idleStart := time.Now()
	if err := sleep(ctx, *idle); err != nil {
		return err
	}
	cpuAfter, _, err := processUsage()
	if err != nil {
		return err
	}
	r.IdleSeconds = time.Since(idleStart).Seconds()
	if r.IdleSeconds > 0 {
		r.IdleCPUPercent = (cpuAfter - cpuBefore).Seconds() / r.IdleSeconds * 100
	}
	r.IdleEvents = drainEvents(s)
	if err := s.Err(); err != nil {
		return err
	}
	if r.IdleEvents != 0 {
		return fmt.Errorf("idle published %d unexpected events", r.IdleEvents)
	}
	for sample := 0; sample < *samples; sample++ {
		copy(data, fmt.Sprintf("%08d", sample))
		hash := sha256.Sum256(data)
		wantHash := hex.EncodeToString(hash[:])
		for i := 0; i < *burst; i++ {
			if err := os.WriteFile(paths[i], data, 0600); err != nil {
				return err
			}
		}
		written := time.Now()
		if err := awaitChanges(ctx, s, *burst, wantHash); err != nil {
			return err
		}
		r.LatencyMS = append(r.LatencyMS, elapsedMS(written))
		drainEvents(s)
	}
	if len(r.LatencyMS) > 0 {
		ordered := append([]float64(nil), r.LatencyMS...)
		sort.Float64s(ordered)
		r.P95MS = ordered[(95*len(ordered)+99)/100-1]
		r.AfterNominalDebounceP95MS = max(0, r.P95MS-r.DebounceMS)
		key, err := filepath.Rel(root, paths[0])
		if err != nil {
			return err
		}
		start := time.Now()
		for {
			result, err := s.GetDiff(ctx, filepath.ToSlash(key))
			if errors.Is(err, changes.ErrStale) && time.Since(start) < 10*time.Second {
				if err := sleep(ctx, 10*time.Millisecond); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			r.DiffStatus, r.DiffMS = result.Status, elapsedMS(start)
			break
		}
	}
	if err := s.Close(); err != nil {
		return err
	}
	for i := 0; i < *cycles; i++ {
		p, err := app.Prepare(ctx, root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
		if err != nil {
			return err
		}
		next, err := app.StartSession(ctx, p)
		if err != nil {
			return err
		}
		if err := next.Pause(ctx); err != nil {
			_ = next.Close()
			return err
		}
		if err := next.Close(); err != nil {
			return err
		}
	}
	stopProfile()
	// Allow runtime/context callbacks to retire; all application owners were joined by Close.
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > r.GoroutinesBefore+2 && time.Now().Before(deadline) {
		runtime.GC()
		if err := sleep(ctx, 10*time.Millisecond); err != nil {
			return err
		}
	}
	r.GoroutinesAfter, r.FDsAfter = runtime.NumGoroutine(), countFDs()
	entries, err := os.ReadDir(cache)
	if err != nil {
		return err
	}
	r.CacheEntriesAfter = len(entries)
	_, r.PeakRSSBytes, err = processUsage()
	if err != nil {
		return err
	}
	// A runtime poll descriptor may be initialized lazily. Do not allow per-cycle growth.
	r.Passed = r.GoroutinesAfter <= r.GoroutinesBefore+2 && r.FDsBefore >= 0 && r.FDsAfter >= 0 && r.FDsAfter <= r.FDsBefore+2 && r.CacheEntriesAfter == 0
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(r); err != nil {
		return err
	}
	if !r.Passed {
		return errors.New("resource cleanup assertion failed")
	}
	return nil
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func awaitChanges(ctx context.Context, s *app.Session, count int, hash string) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		view := s.ChangeState()
		ok := len(view.Changes) == count
		for _, item := range view.Changes {
			ok = ok && item.Kind == changes.Modified && item.After != nil && item.After.Hash == hash
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.Done():
			return fmt.Errorf("session stopped: %w", s.Err())
		case <-deadline.C:
			return fmt.Errorf("changes did not converge to %d exact hashes", count)
		case <-tick.C:
		}
	}
}

func drainEvents(s *app.Session) int {
	count := 0
	for {
		select {
		case _, ok := <-s.Events():
			if !ok {
				return count
			}
			count++
		default:
			return count
		}
	}
}

func countFDs() int {
	return testutil.OpenDescriptors()
}

func cpuProfile(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	stopped := false
	return func() {
		if !stopped {
			stopped = true
			pprof.StopCPUProfile()
			_ = file.Close()
		}
	}, nil
}

func writeProfile(path, name string, debug int) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = pprof.Lookup(name).WriteTo(file, debug)
	return errors.Join(err, file.Close())
}
