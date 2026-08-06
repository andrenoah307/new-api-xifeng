package common

import (
	"bufio"
	"fmt"
	"math/bits"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	cgroupV1                  = 1
	cgroupV2                  = 2
	cgroupMemorySamplePeriod  = 2 * time.Second
	cgroupV1UnlimitedSentinel = uint64(9223372036854771712)
	procSelfCgroupPath        = "/proc/self/cgroup"
	procSelfMountInfoPath     = "/proc/self/mountinfo"
	cgroupMemoryLogInterval   = 60 * time.Second
)

type cgroupMemorySample struct {
	usageBytes uint64 // cgroup working set: usage minus inactive file cache
	limitBytes uint64
	available  bool
}

// CgroupMemoryStatus is the latest sampled cgroup working-set status together
// with the configured relay breaker state.
type CgroupMemoryStatus struct {
	Available        bool   `json:"available"`
	UsageBytes       uint64 `json:"usage_bytes"`
	LimitBytes       uint64 `json:"limit_bytes"`
	UsagePermille    uint64 `json:"usage_permille"`
	HighPercent      int    `json:"high_percent"`
	LowPercent       int    `json:"low_percent"`
	Tripped          bool   `json:"tripped"`
	TrippedSinceUnix int    `json:"tripped_since_unix"`
	TripCount        uint64 `json:"trip_count"`
	ForcedResetCount uint64 `json:"forced_reset_count"`
	MaxTripSeconds   int    `json:"max_trip_seconds"`
	Disarmed         bool   `json:"disarmed"`
}

type cgroupMemoryRawStatus struct {
	usageBytes    atomic.Uint64
	limitBytes    atomic.Uint64
	usagePermille atomic.Uint64
	available     atomic.Bool
}

func (s *cgroupMemoryRawStatus) store(sample cgroupMemorySample) {
	s.usageBytes.Store(sample.usageBytes)
	s.limitBytes.Store(sample.limitBytes)
	s.usagePermille.Store(cgroupMemoryUsagePermille(sample.usageBytes, sample.limitBytes))
	s.available.Store(sample.available)
}

type cgroupMemoryBreaker struct {
	usagePermille        atomic.Uint64
	available            atomic.Bool
	tripped              atomic.Bool
	trippedSinceUnixNano atomic.Int64
	disarmed             atomic.Bool
	tripCount            atomic.Uint64
	forcedResetCount     atomic.Uint64
	lastTransitionLog    atomic.Int64
}

func (b *cgroupMemoryBreaker) update(sample cgroupMemorySample, highPercent uint64, lowPercent uint64, maxTripSeconds int) {
	if !sample.available || sample.limitBytes == 0 || highPercent == 0 {
		b.available.Store(false)
		wasTripped := b.tripped.Swap(false)
		b.trippedSinceUnixNano.Store(0)
		b.disarmed.Store(false)
		b.usagePermille.Store(0)
		if wasTripped {
			b.logTransition(sample, cgroupMemoryUsagePermille(sample.usageBytes, sample.limitBytes), highPercent, lowPercent, false)
		}
		return
	}

	usagePermille := cgroupMemoryUsagePermille(sample.usageBytes, sample.limitBytes)
	b.usagePermille.Store(usagePermille)
	b.available.Store(true)
	if b.disarmed.Load() {
		b.tripped.Store(false)
		b.trippedSinceUnixNano.Store(0)
		if usagePermille <= lowPercent*10 {
			b.disarmed.Store(false)
			SysLog(fmt.Sprintf(
				"cgroup memory breaker state changed: usage=%d limit=%d permille=%d high=%d low=%d tripped=false disarmed=false rearmed=true",
				sample.usageBytes,
				sample.limitBytes,
				usagePermille,
				highPercent,
				lowPercent,
			))
		}
		return
	}

	if b.tripped.Load() && maxTripSeconds > 0 {
		trippedSince := b.trippedSinceUnixNano.Load()
		now := cgroupMemoryNowFunc().UnixNano()
		if trippedSince > 0 && now >= trippedSince && (now-trippedSince)/int64(time.Second) >= int64(maxTripSeconds) {
			b.tripped.Store(false)
			b.trippedSinceUnixNano.Store(0)
			b.disarmed.Store(true)
			b.forcedResetCount.Add(1)
			SysLog(fmt.Sprintf(
				"cgroup memory breaker state changed: usage=%d limit=%d permille=%d high=%d low=%d tripped=false disarmed=true forced_reset=true",
				sample.usageBytes,
				sample.limitBytes,
				usagePermille,
				highPercent,
				lowPercent,
			))
			return
		}
	}

	nextTripped := b.tripped.Load()
	if usagePermille >= highPercent*10 {
		nextTripped = true
	} else if usagePermille <= lowPercent*10 {
		nextTripped = false
	}
	previousTripped := b.tripped.Swap(nextTripped)
	if previousTripped != nextTripped {
		if nextTripped {
			b.trippedSinceUnixNano.Store(cgroupMemoryNowFunc().UnixNano())
			b.tripCount.Add(1)
		} else {
			b.trippedSinceUnixNano.Store(0)
		}
		b.logTransition(sample, usagePermille, highPercent, lowPercent, nextTripped)
	} else if !nextTripped {
		b.trippedSinceUnixNano.Store(0)
	}
}

func (b *cgroupMemoryBreaker) logTransition(sample cgroupMemorySample, usagePermille uint64, highPercent uint64, lowPercent uint64, tripped bool) {
	now := cgroupMemoryNowFunc().UnixNano()
	for {
		last := b.lastTransitionLog.Load()
		if last != 0 && now-last < int64(cgroupMemoryLogInterval) {
			return
		}
		if b.lastTransitionLog.CompareAndSwap(last, now) {
			SysLog(fmt.Sprintf(
				"cgroup memory breaker state changed: usage=%d limit=%d permille=%d high=%d low=%d tripped=%t",
				sample.usageBytes,
				sample.limitBytes,
				usagePermille,
				highPercent,
				lowPercent,
				tripped,
			))
			return
		}
	}
}

func (b *cgroupMemoryBreaker) isTripped() bool {
	return b.available.Load() && b.tripped.Load()
}

func cgroupMemoryUsagePermille(usageBytes uint64, limitBytes uint64) uint64 {
	if limitBytes == 0 {
		return 0
	}
	if usageBytes >= limitBytes {
		return 1000
	}
	high, low := bits.Mul64(usageBytes, 1000)
	usagePermille, _ := bits.Div64(high, low, limitBytes)
	return usagePermille
}

func normalizeRelayMemoryBreakerThresholds(highPercent int, lowPercent int) (int, bool) {
	if lowPercent < 0 {
		return 0, highPercent > 0
	}
	if highPercent <= 0 || lowPercent < highPercent {
		return lowPercent, false
	}
	return max(0, highPercent-10), true
}

type cgroupMembership struct {
	version    int
	cgroupPath string
}

type cgroupMount struct {
	version    int
	root       string
	mountPoint string
}

func readCgroupMemorySample(cgroupFile string, mountInfoFile string) cgroupMemorySample {
	cgroupData, err := os.ReadFile(cgroupFile)
	if err != nil {
		return cgroupMemorySample{}
	}
	mountInfoData, err := os.ReadFile(mountInfoFile)
	if err != nil {
		return cgroupMemorySample{}
	}

	memberships := parseCgroupMemberships(string(cgroupData))
	mounts := parseCgroupMounts(string(mountInfoData))
	for _, membership := range memberships {
		memoryDir := ""
		bestRootLength := -1
		for _, mount := range mounts {
			if mount.version != membership.version {
				continue
			}
			candidate, ok := resolveCgroupMemoryDir(mount, membership.cgroupPath)
			if !ok || len(mount.root) <= bestRootLength {
				continue
			}
			memoryDir = candidate
			bestRootLength = len(mount.root)
		}
		if memoryDir == "" {
			continue
		}

		sample := readCgroupMemoryFiles(memoryDir, membership.version)
		if sample.available {
			return sample
		}
	}
	return cgroupMemorySample{}
}

func parseCgroupMemberships(data string) []cgroupMembership {
	var v2Memberships []cgroupMembership
	var v1Memberships []cgroupMembership
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) != 3 || !strings.HasPrefix(fields[2], "/") {
			continue
		}
		if fields[1] == "" {
			v2Memberships = append(v2Memberships, cgroupMembership{version: cgroupV2, cgroupPath: path.Clean(fields[2])})
			continue
		}
		for _, controller := range strings.Split(fields[1], ",") {
			if controller == "memory" {
				v1Memberships = append(v1Memberships, cgroupMembership{version: cgroupV1, cgroupPath: path.Clean(fields[2])})
				break
			}
		}
	}
	return append(v2Memberships, v1Memberships...)
}

func parseCgroupMounts(data string) []cgroupMount {
	var mounts []cgroupMount
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 5 || len(fields) <= separator+3 {
			continue
		}

		version := 0
		switch fields[separator+1] {
		case "cgroup2":
			version = cgroupV2
		case "cgroup":
			for _, field := range fields[separator+2:] {
				for _, option := range strings.Split(field, ",") {
					if option == "memory" {
						version = cgroupV1
						break
					}
				}
				if version != 0 {
					break
				}
			}
		}
		if version == 0 {
			continue
		}

		mounts = append(mounts, cgroupMount{
			version:    version,
			root:       path.Clean(unescapeMountInfoPath(fields[3])),
			mountPoint: filepath.Clean(unescapeMountInfoPath(fields[4])),
		})
	}
	return mounts
}

func unescapeMountInfoPath(value string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(value)
}

func resolveCgroupMemoryDir(mount cgroupMount, cgroupPath string) (string, bool) {
	cgroupPath = path.Clean(cgroupPath)
	mountRoot := path.Clean(mount.root)
	var relativePath string
	switch {
	case mountRoot == "/":
		relativePath = strings.TrimPrefix(cgroupPath, "/")
	case cgroupPath == mountRoot:
		relativePath = ""
	case strings.HasPrefix(cgroupPath, mountRoot+"/"):
		relativePath = strings.TrimPrefix(cgroupPath, mountRoot+"/")
	default:
		return "", false
	}
	return filepath.Join(mount.mountPoint, filepath.FromSlash(relativePath)), true
}

func readCgroupMemoryFiles(memoryDir string, version int) cgroupMemorySample {
	usageFile := "memory.current"
	limitFile := "memory.max"
	inactiveFileField := "inactive_file"
	if version == cgroupV1 {
		usageFile = "memory.usage_in_bytes"
		limitFile = "memory.limit_in_bytes"
		inactiveFileField = "total_inactive_file"
	}

	usageData, err := os.ReadFile(filepath.Join(memoryDir, usageFile))
	if err != nil {
		return cgroupMemorySample{}
	}
	limitData, err := os.ReadFile(filepath.Join(memoryDir, limitFile))
	if err != nil {
		return cgroupMemorySample{}
	}

	limitValue := strings.TrimSpace(string(limitData))
	if version == cgroupV2 && limitValue == "max" {
		return cgroupMemorySample{}
	}
	usageBytes, err := strconv.ParseUint(strings.TrimSpace(string(usageData)), 10, 64)
	if err != nil {
		return cgroupMemorySample{}
	}
	limitBytes, err := strconv.ParseUint(limitValue, 10, 64)
	if err != nil || limitBytes == 0 {
		return cgroupMemorySample{}
	}
	if version == cgroupV1 && limitBytes >= cgroupV1UnlimitedSentinel {
		return cgroupMemorySample{}
	}

	statData, err := os.ReadFile(filepath.Join(memoryDir, "memory.stat"))
	if err != nil {
		return cgroupMemorySample{}
	}
	inactiveFileBytes := uint64(0)
	inactiveFileFound := false
	scanner := bufio.NewScanner(strings.NewReader(string(statData)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[0] != inactiveFileField {
			continue
		}
		inactiveFileBytes, err = strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return cgroupMemorySample{}
		}
		inactiveFileFound = true
		break
	}
	if scanner.Err() != nil || !inactiveFileFound {
		return cgroupMemorySample{}
	}

	workingSetBytes := uint64(0)
	if usageBytes > inactiveFileBytes {
		workingSetBytes = usageBytes - inactiveFileBytes
	}
	return cgroupMemorySample{usageBytes: workingSetBytes, limitBytes: limitBytes, available: true}
}

type cgroupMemorySampler struct {
	cgroupFile    string
	mountInfoFile string
	highPercent   uint64
	lowPercent    uint64
	breaker       *cgroupMemoryBreaker
	rawStatus     *cgroupMemoryRawStatus
}

func (s *cgroupMemorySampler) sample() {
	sample := readCgroupMemorySample(s.cgroupFile, s.mountInfoFile)
	rawStatus := s.rawStatus
	if rawStatus == nil {
		rawStatus = &relayCgroupMemoryRawStatus
	}
	rawStatus.store(sample)
	s.breaker.update(sample, s.highPercent, s.lowPercent, RelayMemoryBreakerMaxTripSeconds)
}

func (s *cgroupMemorySampler) run(ticks <-chan time.Time) {
	for range ticks {
		s.sample()
	}
}

var (
	relayCgroupMemoryBreaker            cgroupMemoryBreaker
	relayCgroupMemoryRawStatus          cgroupMemoryRawStatus
	cgroupMemorySamplerOnce             sync.Once
	relayMemoryBreakerConfigWarningOnce sync.Once
	cgroupMemoryNowFunc                 = time.Now
)

func shouldInitCgroupMemorySampler(highPercent int, runningInContainer bool, monitorConfig PerformanceMonitorConfig) bool {
	return highPercent > 0 || (runningInContainer && monitorConfig.Enabled && monitorConfig.MemoryThreshold > 0)
}

// InitCgroupMemorySampler starts one process-wide sampler. Requests only read the
// atomic breaker state; cgroup files are never opened on the request path.
func InitCgroupMemorySampler() {
	if !shouldInitCgroupMemorySampler(RelayMemoryBreakerHighPercent, IsRunningInContainer(), GetPerformanceMonitorConfig()) {
		return
	}
	cgroupMemorySamplerOnce.Do(func() {
		sampler := &cgroupMemorySampler{
			cgroupFile:    procSelfCgroupPath,
			mountInfoFile: procSelfMountInfoPath,
			highPercent:   uint64(RelayMemoryBreakerHighPercent),
			lowPercent:    uint64(RelayMemoryBreakerLowPercent),
			breaker:       &relayCgroupMemoryBreaker,
			rawStatus:     &relayCgroupMemoryRawStatus,
		}
		sampler.sample()
		go func() {
			ticker := time.NewTicker(cgroupMemorySamplePeriod)
			defer ticker.Stop()
			sampler.run(ticker.C)
		}()
	})
}

// IsCgroupMemoryPressure reports the last sampled breaker state. The sampler uses
// the kubelet-style working set (usage minus inactive file cache), retaining cgroup
// memory outside the Go heap without treating reclaimable page cache as pressure.
func IsCgroupMemoryPressure() bool {
	return relayCgroupMemoryBreaker.isTripped()
}

// GetCgroupMemoryStatus returns the last raw sample. It is independent of the
// relay breaker thresholds so the dashboard can show real cgroup usage even
// when the breaker is disabled.
func GetCgroupMemoryStatus() CgroupMemoryStatus {
	tripped := relayCgroupMemoryBreaker.isTripped()
	trippedSinceUnix := 0
	if tripped {
		trippedSinceUnix = int(relayCgroupMemoryBreaker.trippedSinceUnixNano.Load() / int64(time.Second))
	}
	return CgroupMemoryStatus{
		Available:        relayCgroupMemoryRawStatus.available.Load(),
		UsageBytes:       relayCgroupMemoryRawStatus.usageBytes.Load(),
		LimitBytes:       relayCgroupMemoryRawStatus.limitBytes.Load(),
		UsagePermille:    relayCgroupMemoryRawStatus.usagePermille.Load(),
		HighPercent:      RelayMemoryBreakerHighPercent,
		LowPercent:       RelayMemoryBreakerLowPercent,
		Tripped:          tripped,
		TrippedSinceUnix: trippedSinceUnix,
		TripCount:        relayCgroupMemoryBreaker.tripCount.Load(),
		ForcedResetCount: relayCgroupMemoryBreaker.forcedResetCount.Load(),
		MaxTripSeconds:   RelayMemoryBreakerMaxTripSeconds,
		Disarmed:         relayCgroupMemoryBreaker.disarmed.Load(),
	}
}
