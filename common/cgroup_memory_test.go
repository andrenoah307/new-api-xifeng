package common

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cgroupMemoryFixture struct {
	cgroupFile    string
	mountInfoFile string
	memoryDir     string
}

func newCgroupMemoryFixture(t *testing.T, version int) cgroupMemoryFixture {
	t.Helper()

	tempDir := t.TempDir()
	cgroupFile := filepath.Join(tempDir, "self.cgroup")
	mountInfoFile := filepath.Join(tempDir, "self.mountinfo")
	mountPoint := filepath.Join(tempDir, fmt.Sprintf("cgroup-v%d", version))
	require.NoError(t, os.MkdirAll(mountPoint, 0o755))

	var cgroupContent string
	var mountInfoContent string
	var memoryDir string
	switch version {
	case 2:
		cgroupContent = "0::/kubepods.slice/pod123/container456\n"
		mountInfoContent = fmt.Sprintf("31 23 0:28 /kubepods.slice/pod123 %s rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n", mountPoint)
		memoryDir = filepath.Join(mountPoint, "container456")
	case 1:
		cgroupContent = "8:cpu,cpuacct:/unrelated\n7:memory:/docker/parent/container456\n"
		mountInfoContent = fmt.Sprintf("42 31 0:37 /docker/parent %s rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,memory\n", mountPoint)
		memoryDir = filepath.Join(mountPoint, "container456")
	default:
		t.Fatalf("unsupported fixture cgroup version %d", version)
	}

	require.NoError(t, os.WriteFile(cgroupFile, []byte(cgroupContent), 0o600))
	require.NoError(t, os.WriteFile(mountInfoFile, []byte(mountInfoContent), 0o600))
	require.NoError(t, os.MkdirAll(memoryDir, 0o755))
	return cgroupMemoryFixture{cgroupFile: cgroupFile, mountInfoFile: mountInfoFile, memoryDir: memoryDir}
}

func TestReadCgroupMemorySampleUsesWorkingSetForV2AndV1NestedMountRoots(t *testing.T) {
	tests := []struct {
		name      string
		version   int
		usageFile string
		limitFile string
		statValue string
	}{
		{
			name: "cgroup v2 subtracts inactive_file", version: 2,
			usageFile: "memory.current", limitFile: "memory.max",
			statValue: "anon 30\nfile 51\ninactive_file 31\n",
		},
		{
			name: "cgroup v1 subtracts total_inactive_file", version: 1,
			usageFile: "memory.usage_in_bytes", limitFile: "memory.limit_in_bytes",
			statValue: "cache 51\ninactive_file 7\ntotal_inactive_file 31\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCgroupMemoryFixture(t, test.version)
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.usageFile), []byte("81\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.limitFile), []byte("100\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte(test.statValue), 0o600))

			sample := readCgroupMemorySample(fixture.cgroupFile, fixture.mountInfoFile)

			require.True(t, sample.available)
			assert.EqualValues(t, 50, sample.usageBytes)
			assert.EqualValues(t, 100, sample.limitBytes)
		})
	}
}

func TestReadCgroupMemorySampleWorkingSetClampsInactiveFileUnderflow(t *testing.T) {
	tests := []struct {
		name      string
		version   int
		usageFile string
		limitFile string
		statValue string
	}{
		{
			name: "cgroup v2", version: 2,
			usageFile: "memory.current", limitFile: "memory.max", statValue: "inactive_file 30\n",
		},
		{
			name: "cgroup v1", version: 1,
			usageFile: "memory.usage_in_bytes", limitFile: "memory.limit_in_bytes", statValue: "total_inactive_file 30\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCgroupMemoryFixture(t, test.version)
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.usageFile), []byte("20\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.limitFile), []byte("100\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte(test.statValue), 0o600))

			sample := readCgroupMemorySample(fixture.cgroupFile, fixture.mountInfoFile)

			require.True(t, sample.available)
			assert.Zero(t, sample.usageBytes)
			assert.EqualValues(t, 100, sample.limitBytes)
		})
	}
}

func TestReadCgroupMemorySampleFailsOpenWhenLimitIsUnlimitedOrFilesAreMissing(t *testing.T) {
	tests := []struct {
		name       string
		version    int
		usageFile  string
		limitFile  string
		limitValue string
		omitUsage  bool
		omitLimit  bool
		statValue  string
		omitStat   bool
	}{
		{
			name: "v2 max means unlimited", version: 2,
			usageFile: "memory.current", limitFile: "memory.max", limitValue: "max\n",
		},
		{
			name: "v1 kernel sentinel means unlimited", version: 1,
			usageFile: "memory.usage_in_bytes", limitFile: "memory.limit_in_bytes", limitValue: "9223372036854771712\n",
		},
		{
			name: "usage file missing", version: 2,
			usageFile: "memory.current", limitFile: "memory.max", limitValue: "100\n", omitUsage: true,
		},
		{
			name: "limit file missing", version: 1,
			usageFile: "memory.usage_in_bytes", limitFile: "memory.limit_in_bytes", omitLimit: true,
		},
		{
			name: "memory stat file missing", version: 2,
			usageFile: "memory.current", limitFile: "memory.max", limitValue: "100\n", omitStat: true,
		},
		{
			name: "v1 memory stat target missing", version: 1,
			usageFile: "memory.usage_in_bytes", limitFile: "memory.limit_in_bytes", limitValue: "100\n", statValue: "inactive_file 20\n",
		},
		{
			name: "v2 memory stat target malformed", version: 2,
			usageFile: "memory.current", limitFile: "memory.max", limitValue: "100\n", statValue: "inactive_file not-a-number\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCgroupMemoryFixture(t, test.version)
			if !test.omitUsage {
				require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.usageFile), []byte("80\n"), 0o600))
			}
			if !test.omitLimit {
				require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, test.limitFile), []byte(test.limitValue), 0o600))
			}
			if !test.omitStat {
				statValue := test.statValue
				if statValue == "" && test.version == 2 {
					statValue = "inactive_file 10\n"
				} else if statValue == "" {
					statValue = "total_inactive_file 10\n"
				}
				require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte(statValue), 0o600))
			}

			sample := readCgroupMemorySample(fixture.cgroupFile, fixture.mountInfoFile)

			assert.False(t, sample.available)
			assert.Zero(t, sample.usageBytes)
			assert.Zero(t, sample.limitBytes)
		})
	}
}

func TestReadCgroupMemorySampleFailsOpenWhenProcMetadataCannotBeResolved(t *testing.T) {
	tests := []struct {
		name          string
		cgroupContent string
		mountContent  string
	}{
		{name: "malformed cgroup", cgroupContent: "not-a-cgroup-entry\n", mountContent: ""},
		{name: "no matching mount", cgroupContent: "0::/container\n", mountContent: "31 23 0:28 / /tmp/not-cgroup rw - tmpfs tmpfs rw\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir := t.TempDir()
			cgroupFile := filepath.Join(tempDir, "self.cgroup")
			mountInfoFile := filepath.Join(tempDir, "self.mountinfo")
			require.NoError(t, os.WriteFile(cgroupFile, []byte(test.cgroupContent), 0o600))
			require.NoError(t, os.WriteFile(mountInfoFile, []byte(test.mountContent), 0o600))

			sample := readCgroupMemorySample(cgroupFile, mountInfoFile)

			assert.False(t, sample.available)
		})
	}

	missing := readCgroupMemorySample(filepath.Join(t.TempDir(), "missing-cgroup"), filepath.Join(t.TempDir(), "missing-mountinfo"))
	assert.False(t, missing.available)

	tempDir := t.TempDir()
	cgroupFile := filepath.Join(tempDir, "self.cgroup")
	require.NoError(t, os.WriteFile(cgroupFile, []byte("0::/container\n"), 0o600))
	missingMountInfo := readCgroupMemorySample(cgroupFile, filepath.Join(tempDir, "missing-mountinfo"))
	assert.False(t, missingMountInfo.available)
}

func TestCgroupMemoryBreakerHysteresis(t *testing.T) {
	breaker := &cgroupMemoryBreaker{}
	tests := []struct {
		name        string
		usage       uint64
		limit       uint64
		available   bool
		wantTripped bool
	}{
		{name: "below high remains closed", usage: 79, limit: 100, available: true, wantTripped: false},
		{name: "at high trips", usage: 80, limit: 100, available: true, wantTripped: true},
		{name: "between high and low stays tripped", usage: 75, limit: 100, available: true, wantTripped: true},
		{name: "at low recovers", usage: 70, limit: 100, available: true, wantTripped: false},
		{name: "above high trips again", usage: 90, limit: 100, available: true, wantTripped: true},
		{name: "unavailable sample fails open", available: false, wantTripped: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			breaker.update(cgroupMemorySample{
				usageBytes: test.usage,
				limitBytes: test.limit,
				available:  test.available,
			}, 80, 70, 0)

			assert.Equal(t, test.wantTripped, breaker.isTripped())
		})
	}
}

func TestNormalizeRelayMemoryBreakerThresholds(t *testing.T) {
	tests := []struct {
		name        string
		high        int
		low         int
		wantLow     int
		wantInvalid bool
	}{
		{name: "disabled high ignores low", high: 0, low: 75, wantLow: 75, wantInvalid: false},
		{name: "valid hysteresis", high: 85, low: 75, wantLow: 75, wantInvalid: false},
		{name: "equal thresholds fall back", high: 85, low: 85, wantLow: 75, wantInvalid: true},
		{name: "low above high falls back", high: 80, low: 90, wantLow: 70, wantInvalid: true},
		{name: "small high clamps fallback to zero", high: 8, low: 75, wantLow: 0, wantInvalid: true},
		{name: "negative low clamps to zero", high: 80, low: -5, wantLow: 0, wantInvalid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			low, invalid := normalizeRelayMemoryBreakerThresholds(test.high, test.low)
			assert.Equal(t, test.wantLow, low)
			assert.Equal(t, test.wantInvalid, invalid)
		})
	}
}

func TestCgroupMemoryBreakerSmallHighFallbackCanRecover(t *testing.T) {
	tests := []struct {
		name          string
		high          int
		configuredLow int
		tripUsage     uint64
		recoveryUsage uint64
	}{
		{name: "high below fallback delta", high: 8, configuredLow: 75, tripUsage: 8, recoveryUsage: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			low, invalid := normalizeRelayMemoryBreakerThresholds(test.high, test.configuredLow)
			require.True(t, invalid)
			require.Zero(t, low)

			breaker := &cgroupMemoryBreaker{}
			breaker.update(cgroupMemorySample{usageBytes: test.tripUsage, limitBytes: 100, available: true}, uint64(test.high), uint64(low), 0)
			require.True(t, breaker.isTripped())

			breaker.update(cgroupMemorySample{usageBytes: test.recoveryUsage, limitBytes: 100, available: true}, uint64(test.high), uint64(low), 0)
			assert.False(t, breaker.isTripped())
		})
	}
}

func TestCgroupMemoryUsagePermille(t *testing.T) {
	tests := []struct {
		name  string
		usage uint64
		limit uint64
		want  uint64
	}{
		{name: "zero limit", usage: 1, limit: 0, want: 0},
		{name: "fraction", usage: 81, limit: 100, want: 810},
		{name: "at limit", usage: 100, limit: 100, want: 1000},
		{name: "over limit is capped", usage: 120, limit: 100, want: 1000},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, cgroupMemoryUsagePermille(test.usage, test.limit))
		})
	}
}

func TestResolveCgroupMemoryDir(t *testing.T) {
	mountPoint := filepath.Join(t.TempDir(), "memory")
	tests := []struct {
		name       string
		mountRoot  string
		cgroupPath string
		wantPath   string
		wantOK     bool
	}{
		{name: "root mount", mountRoot: "/", cgroupPath: "/pod/container", wantPath: filepath.Join(mountPoint, "pod/container"), wantOK: true},
		{name: "exact delegated root", mountRoot: "/pod/container", cgroupPath: "/pod/container", wantPath: mountPoint, wantOK: true},
		{name: "nested delegated root", mountRoot: "/pod", cgroupPath: "/pod/container", wantPath: filepath.Join(mountPoint, "container"), wantOK: true},
		{name: "prefix must end at boundary", mountRoot: "/pod", cgroupPath: "/podcast/container", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, ok := resolveCgroupMemoryDir(cgroupMount{root: test.mountRoot, mountPoint: mountPoint}, test.cgroupPath)
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.wantPath, resolved)
		})
	}
}

func TestCgroupMemorySamplerUpdatesOnlyOnSampleTicks(t *testing.T) {
	fixture := newCgroupMemoryFixture(t, 2)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.current"), []byte("85\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.max"), []byte("100\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte("inactive_file 0\n"), 0o600))

	breaker := &cgroupMemoryBreaker{}
	rawStatus := &cgroupMemoryRawStatus{}
	sampler := &cgroupMemorySampler{
		cgroupFile:    fixture.cgroupFile,
		mountInfoFile: fixture.mountInfoFile,
		highPercent:   80,
		lowPercent:    70,
		breaker:       breaker,
		rawStatus:     rawStatus,
	}
	ticks := make(chan time.Time, 1)
	ticks <- time.Time{}
	close(ticks)
	sampler.run(ticks)
	assert.True(t, breaker.isTripped())

	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.current"), []byte("65\n"), 0o600))
	sampler.sample()
	assert.False(t, breaker.isTripped())
}

func TestCgroupMemorySamplerUsesConfiguredMaxTripDuration(t *testing.T) {
	fixture := newCgroupMemoryFixture(t, 2)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.current"), []byte("85\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.max"), []byte("100\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte("inactive_file 0\n"), 0o600))

	previousNowFunc := cgroupMemoryNowFunc
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	previousWriter := gin.DefaultWriter
	currentTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	cgroupMemoryNowFunc = func() time.Time { return currentTime }
	RelayMemoryBreakerMaxTripSeconds = 300
	gin.DefaultWriter = &bytes.Buffer{}
	t.Cleanup(func() {
		cgroupMemoryNowFunc = previousNowFunc
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
		gin.DefaultWriter = previousWriter
	})

	breaker := &cgroupMemoryBreaker{}
	sampler := &cgroupMemorySampler{
		cgroupFile:    fixture.cgroupFile,
		mountInfoFile: fixture.mountInfoFile,
		highPercent:   80,
		lowPercent:    70,
		breaker:       breaker,
		rawStatus:     &cgroupMemoryRawStatus{},
	}
	sampler.sample()
	require.True(t, breaker.isTripped())

	currentTime = currentTime.Add(300 * time.Second)
	sampler.sample()

	assert.False(t, breaker.isTripped())
	assert.True(t, breaker.disarmed.Load())
	assert.EqualValues(t, 1, breaker.forcedResetCount.Load())
}

func TestCgroupMemoryRawStatusRemainsAvailableWhenBreakerDisabled(t *testing.T) {
	fixture := newCgroupMemoryFixture(t, 2)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.current"), []byte("60\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.max"), []byte("100\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.memoryDir, "memory.stat"), []byte("inactive_file 10\n"), 0o600))

	previousStatus := GetCgroupMemoryStatus()
	previousHigh := RelayMemoryBreakerHighPercent
	previousLow := RelayMemoryBreakerLowPercent
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	t.Cleanup(func() {
		relayCgroupMemoryRawStatus.available.Store(previousStatus.Available)
		relayCgroupMemoryRawStatus.usageBytes.Store(previousStatus.UsageBytes)
		relayCgroupMemoryRawStatus.limitBytes.Store(previousStatus.LimitBytes)
		relayCgroupMemoryRawStatus.usagePermille.Store(previousStatus.UsagePermille)
		RelayMemoryBreakerHighPercent = previousHigh
		RelayMemoryBreakerLowPercent = previousLow
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
	})

	RelayMemoryBreakerHighPercent = 0
	RelayMemoryBreakerLowPercent = 75
	RelayMemoryBreakerMaxTripSeconds = 0
	breaker := &cgroupMemoryBreaker{}
	sampler := &cgroupMemorySampler{
		cgroupFile:    fixture.cgroupFile,
		mountInfoFile: fixture.mountInfoFile,
		highPercent:   0,
		lowPercent:    75,
		breaker:       breaker,
	}
	sampler.sample()

	assert.Equal(t, CgroupMemoryStatus{
		Available:     true,
		UsageBytes:    50,
		LimitBytes:    100,
		UsagePermille: 500,
		HighPercent:   0,
		LowPercent:    75,
		Tripped:       false,
	}, GetCgroupMemoryStatus())
	assert.False(t, breaker.available.Load())
}

func TestGetCgroupMemoryStatusBeforeFirstSampleReturnsZeroValues(t *testing.T) {
	previousStatus := GetCgroupMemoryStatus()
	previousHigh := RelayMemoryBreakerHighPercent
	previousLow := RelayMemoryBreakerLowPercent
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	previousBreakerAvailable := relayCgroupMemoryBreaker.available.Load()
	previousBreakerTripped := relayCgroupMemoryBreaker.tripped.Load()
	previousTrippedSince := relayCgroupMemoryBreaker.trippedSinceUnixNano.Load()
	previousDisarmed := relayCgroupMemoryBreaker.disarmed.Load()
	previousTripCount := relayCgroupMemoryBreaker.tripCount.Load()
	previousForcedResetCount := relayCgroupMemoryBreaker.forcedResetCount.Load()
	t.Cleanup(func() {
		relayCgroupMemoryRawStatus.available.Store(previousStatus.Available)
		relayCgroupMemoryRawStatus.usageBytes.Store(previousStatus.UsageBytes)
		relayCgroupMemoryRawStatus.limitBytes.Store(previousStatus.LimitBytes)
		relayCgroupMemoryRawStatus.usagePermille.Store(previousStatus.UsagePermille)
		relayCgroupMemoryBreaker.available.Store(previousBreakerAvailable)
		relayCgroupMemoryBreaker.tripped.Store(previousBreakerTripped)
		relayCgroupMemoryBreaker.trippedSinceUnixNano.Store(previousTrippedSince)
		relayCgroupMemoryBreaker.disarmed.Store(previousDisarmed)
		relayCgroupMemoryBreaker.tripCount.Store(previousTripCount)
		relayCgroupMemoryBreaker.forcedResetCount.Store(previousForcedResetCount)
		RelayMemoryBreakerHighPercent = previousHigh
		RelayMemoryBreakerLowPercent = previousLow
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
	})

	relayCgroupMemoryRawStatus.available.Store(false)
	relayCgroupMemoryRawStatus.usageBytes.Store(0)
	relayCgroupMemoryRawStatus.limitBytes.Store(0)
	relayCgroupMemoryRawStatus.usagePermille.Store(0)
	relayCgroupMemoryBreaker.available.Store(false)
	relayCgroupMemoryBreaker.tripped.Store(false)
	relayCgroupMemoryBreaker.trippedSinceUnixNano.Store(0)
	relayCgroupMemoryBreaker.disarmed.Store(false)
	relayCgroupMemoryBreaker.tripCount.Store(0)
	relayCgroupMemoryBreaker.forcedResetCount.Store(0)
	RelayMemoryBreakerHighPercent = 0
	RelayMemoryBreakerLowPercent = 0
	RelayMemoryBreakerMaxTripSeconds = 0

	assert.Equal(t, CgroupMemoryStatus{}, GetCgroupMemoryStatus())
}

func TestCgroupMemoryBreakerTransitionLogsAreRateLimited(t *testing.T) {
	previousNowFunc := cgroupMemoryNowFunc
	previousWriter := gin.DefaultWriter
	currentTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	cgroupMemoryNowFunc = func() time.Time { return currentTime }
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	t.Cleanup(func() {
		cgroupMemoryNowFunc = previousNowFunc
		gin.DefaultWriter = previousWriter
	})

	breaker := &cgroupMemoryBreaker{}
	breaker.update(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, 80, 70, 0)
	require.True(t, breaker.isTripped())
	assert.Equal(t, 1, strings.Count(logs.String(), "cgroup memory breaker state changed"))
	assert.Contains(t, logs.String(), "tripped=true")

	breaker.update(cgroupMemorySample{usageBytes: 65, limitBytes: 100, available: true}, 80, 70, 0)
	require.False(t, breaker.isTripped())
	breaker.update(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, 80, 70, 0)
	require.True(t, breaker.isTripped())
	assert.Equal(t, 1, strings.Count(logs.String(), "cgroup memory breaker state changed"))

	currentTime = currentTime.Add(60 * time.Second)
	breaker.update(cgroupMemorySample{usageBytes: 65, limitBytes: 100, available: true}, 80, 70, 0)
	require.False(t, breaker.isTripped())
	assert.Equal(t, 2, strings.Count(logs.String(), "cgroup memory breaker state changed"))
	assert.Contains(t, logs.String(), "usage=65 limit=100 permille=650 high=80 low=70 tripped=false")
}

func TestCgroupMemoryBreakerMaxTripDisabledPreservesTrippedState(t *testing.T) {
	previousNowFunc := cgroupMemoryNowFunc
	currentTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	cgroupMemoryNowFunc = func() time.Time { return currentTime }
	t.Cleanup(func() {
		cgroupMemoryNowFunc = previousNowFunc
	})

	breaker := &cgroupMemoryBreaker{}
	breaker.update(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, 80, 70, 0)
	require.True(t, breaker.isTripped())
	tripStartedAt := breaker.trippedSinceUnixNano.Load()
	require.Equal(t, currentTime.UnixNano(), tripStartedAt)

	currentTime = currentTime.Add(24 * time.Hour)
	breaker.update(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, 80, 70, 0)

	assert.True(t, breaker.isTripped())
	assert.False(t, breaker.disarmed.Load())
	assert.Equal(t, tripStartedAt, breaker.trippedSinceUnixNano.Load())
	assert.EqualValues(t, 1, breaker.tripCount.Load())
	assert.Zero(t, breaker.forcedResetCount.Load())
}

func TestCgroupMemoryBreakerMaxTripForcesDisarmUntilLowRecovery(t *testing.T) {
	previousNowFunc := cgroupMemoryNowFunc
	previousWriter := gin.DefaultWriter
	currentTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	cgroupMemoryNowFunc = func() time.Time { return currentTime }
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	t.Cleanup(func() {
		cgroupMemoryNowFunc = previousNowFunc
		gin.DefaultWriter = previousWriter
	})

	breaker := &cgroupMemoryBreaker{}
	highSample := cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}
	breaker.update(highSample, 80, 70, 300)
	require.True(t, breaker.isTripped())
	assert.Equal(t, currentTime.UnixNano(), breaker.trippedSinceUnixNano.Load())
	assert.EqualValues(t, 1, breaker.tripCount.Load())

	currentTime = currentTime.Add(299 * time.Second)
	breaker.update(highSample, 80, 70, 300)
	require.True(t, breaker.isTripped())
	assert.False(t, breaker.disarmed.Load())
	assert.Zero(t, breaker.forcedResetCount.Load())

	currentTime = currentTime.Add(time.Second)
	breaker.lastTransitionLog.Store(currentTime.UnixNano())
	breaker.update(highSample, 80, 70, 300)
	require.False(t, breaker.isTripped())
	assert.True(t, breaker.disarmed.Load())
	assert.Zero(t, breaker.trippedSinceUnixNano.Load())
	assert.EqualValues(t, 1, breaker.tripCount.Load())
	assert.EqualValues(t, 1, breaker.forcedResetCount.Load())
	assert.Contains(t, logs.String(), "usage=85 limit=100 permille=850 high=80 low=70 tripped=false disarmed=true forced_reset=true")

	for range 3 {
		currentTime = currentTime.Add(2 * time.Second)
		breaker.update(highSample, 80, 70, 300)
		assert.False(t, breaker.isTripped())
		assert.True(t, breaker.disarmed.Load())
		assert.EqualValues(t, 1, breaker.tripCount.Load())
		assert.EqualValues(t, 1, breaker.forcedResetCount.Load())
	}

	currentTime = currentTime.Add(2 * time.Second)
	breaker.update(cgroupMemorySample{usageBytes: 70, limitBytes: 100, available: true}, 80, 70, 300)
	require.False(t, breaker.isTripped())
	assert.False(t, breaker.disarmed.Load())
	assert.Zero(t, breaker.trippedSinceUnixNano.Load())
	assert.Contains(t, logs.String(), "usage=70 limit=100 permille=700 high=80 low=70 tripped=false disarmed=false rearmed=true")

	currentTime = currentTime.Add(2 * time.Second)
	breaker.update(highSample, 80, 70, 300)
	require.True(t, breaker.isTripped())
	assert.Equal(t, currentTime.UnixNano(), breaker.trippedSinceUnixNano.Load())
	assert.EqualValues(t, 2, breaker.tripCount.Load())
	assert.EqualValues(t, 1, breaker.forcedResetCount.Load())
}

func TestCgroupMemoryBreakerNormalRecoveryClearsTripStart(t *testing.T) {
	previousNowFunc := cgroupMemoryNowFunc
	currentTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	cgroupMemoryNowFunc = func() time.Time { return currentTime }
	t.Cleanup(func() {
		cgroupMemoryNowFunc = previousNowFunc
	})

	breaker := &cgroupMemoryBreaker{}
	breaker.update(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, 80, 70, 300)
	require.NotZero(t, breaker.trippedSinceUnixNano.Load())

	currentTime = currentTime.Add(120 * time.Second)
	breaker.update(cgroupMemorySample{usageBytes: 70, limitBytes: 100, available: true}, 80, 70, 300)

	assert.False(t, breaker.isTripped())
	assert.False(t, breaker.disarmed.Load())
	assert.Zero(t, breaker.trippedSinceUnixNano.Load())
	assert.EqualValues(t, 1, breaker.tripCount.Load())
	assert.Zero(t, breaker.forcedResetCount.Load())
}

func TestCgroupMemoryBreakerUnavailableStatesClearLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		sample      cgroupMemorySample
		highPercent uint64
	}{
		{name: "sample unavailable", sample: cgroupMemorySample{}, highPercent: 80},
		{name: "zero limit", sample: cgroupMemorySample{usageBytes: 85, available: true}, highPercent: 80},
		{name: "breaker disabled", sample: cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true}, highPercent: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			breaker := &cgroupMemoryBreaker{}
			breaker.available.Store(true)
			breaker.tripped.Store(true)
			breaker.trippedSinceUnixNano.Store(123)
			breaker.disarmed.Store(true)
			breaker.tripCount.Store(4)
			breaker.forcedResetCount.Store(2)

			breaker.update(test.sample, test.highPercent, 70, 300)

			assert.False(t, breaker.available.Load())
			assert.False(t, breaker.isTripped())
			assert.False(t, breaker.disarmed.Load())
			assert.Zero(t, breaker.trippedSinceUnixNano.Load())
			assert.EqualValues(t, 4, breaker.tripCount.Load())
			assert.EqualValues(t, 2, breaker.forcedResetCount.Load())
		})
	}
}

func TestGetCgroupMemoryStatusIncludesBreakerLifecycle(t *testing.T) {
	previousStatus := GetCgroupMemoryStatus()
	previousHigh := RelayMemoryBreakerHighPercent
	previousLow := RelayMemoryBreakerLowPercent
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	previousBreakerAvailable := relayCgroupMemoryBreaker.available.Load()
	previousBreakerTripped := relayCgroupMemoryBreaker.tripped.Load()
	previousTrippedSince := relayCgroupMemoryBreaker.trippedSinceUnixNano.Load()
	previousDisarmed := relayCgroupMemoryBreaker.disarmed.Load()
	previousTripCount := relayCgroupMemoryBreaker.tripCount.Load()
	previousForcedResetCount := relayCgroupMemoryBreaker.forcedResetCount.Load()
	t.Cleanup(func() {
		relayCgroupMemoryRawStatus.available.Store(previousStatus.Available)
		relayCgroupMemoryRawStatus.usageBytes.Store(previousStatus.UsageBytes)
		relayCgroupMemoryRawStatus.limitBytes.Store(previousStatus.LimitBytes)
		relayCgroupMemoryRawStatus.usagePermille.Store(previousStatus.UsagePermille)
		relayCgroupMemoryBreaker.available.Store(previousBreakerAvailable)
		relayCgroupMemoryBreaker.tripped.Store(previousBreakerTripped)
		relayCgroupMemoryBreaker.trippedSinceUnixNano.Store(previousTrippedSince)
		relayCgroupMemoryBreaker.disarmed.Store(previousDisarmed)
		relayCgroupMemoryBreaker.tripCount.Store(previousTripCount)
		relayCgroupMemoryBreaker.forcedResetCount.Store(previousForcedResetCount)
		RelayMemoryBreakerHighPercent = previousHigh
		RelayMemoryBreakerLowPercent = previousLow
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
	})

	tripTime := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	relayCgroupMemoryRawStatus.store(cgroupMemorySample{usageBytes: 85, limitBytes: 100, available: true})
	relayCgroupMemoryBreaker.available.Store(true)
	relayCgroupMemoryBreaker.tripped.Store(true)
	relayCgroupMemoryBreaker.trippedSinceUnixNano.Store(tripTime.UnixNano())
	relayCgroupMemoryBreaker.disarmed.Store(false)
	relayCgroupMemoryBreaker.tripCount.Store(2)
	relayCgroupMemoryBreaker.forcedResetCount.Store(1)
	RelayMemoryBreakerHighPercent = 80
	RelayMemoryBreakerLowPercent = 70
	RelayMemoryBreakerMaxTripSeconds = 300

	assert.Equal(t, CgroupMemoryStatus{
		Available:        true,
		UsageBytes:       85,
		LimitBytes:       100,
		UsagePermille:    850,
		HighPercent:      80,
		LowPercent:       70,
		Tripped:          true,
		TrippedSinceUnix: int(tripTime.Unix()),
		TripCount:        2,
		ForcedResetCount: 1,
		MaxTripSeconds:   300,
		Disarmed:         false,
	}, GetCgroupMemoryStatus())
}

func TestShouldInitCgroupMemorySampler(t *testing.T) {
	tests := []struct {
		name        string
		highPercent int
		inContainer bool
		config      PerformanceMonitorConfig
		want        bool
	}{
		{name: "relay breaker enabled", highPercent: 80, want: true},
		{name: "container memory monitor enabled", inContainer: true, config: PerformanceMonitorConfig{Enabled: true, MemoryThreshold: 90}, want: true},
		{name: "non container monitor", inContainer: false, config: PerformanceMonitorConfig{Enabled: true, MemoryThreshold: 90}, want: false},
		{name: "monitor disabled", inContainer: true, config: PerformanceMonitorConfig{Enabled: false, MemoryThreshold: 90}, want: false},
		{name: "memory threshold disabled", inContainer: true, config: PerformanceMonitorConfig{Enabled: true, MemoryThreshold: 0}, want: false},
		{name: "all gates disabled", inContainer: false, config: PerformanceMonitorConfig{}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, shouldInitCgroupMemorySampler(test.highPercent, test.inContainer, test.config))
		})
	}
}

func TestGetEnvOrDefaultInt64(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue int64
		want         int64
	}{
		{name: "empty uses default", value: "", defaultValue: 17, want: 17},
		{name: "valid int64", value: "4294967296", defaultValue: 17, want: 4294967296},
		{name: "invalid uses default", value: "not-an-integer", defaultValue: 17, want: 17},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TEST_INT64_ENV", test.value)
			assert.Equal(t, test.want, GetEnvOrDefaultInt64("TEST_INT64_ENV", test.defaultValue))
		})
	}
}

func TestInitRelayAdmissionEnvDefaultsKeepAllGatesDisabled(t *testing.T) {
	for _, env := range []string{
		"RELAY_MAX_CONCURRENT_REQUESTS",
		"RELAY_MAX_ACTIVE_BODY_BYTES",
		"RELAY_MEMORY_BREAKER_HIGH_PERCENT",
		"RELAY_MEMORY_BREAKER_LOW_PERCENT",
		"RELAY_MEMORY_BREAKER_MAX_TRIP_SECONDS",
		"RELAY_ADMISSION_RETRY_AFTER_SECONDS",
	} {
		t.Setenv(env, "")
	}

	previousMaxRequests := RelayMaxConcurrentRequests
	previousMaxBodyBytes := RelayMaxActiveBodyBytes
	previousHighPercent := RelayMemoryBreakerHighPercent
	previousLowPercent := RelayMemoryBreakerLowPercent
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	previousRetryAfter := RelayAdmissionRetryAfterSeconds
	t.Cleanup(func() {
		RelayMaxConcurrentRequests = previousMaxRequests
		RelayMaxActiveBodyBytes = previousMaxBodyBytes
		RelayMemoryBreakerHighPercent = previousHighPercent
		RelayMemoryBreakerLowPercent = previousLowPercent
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
		RelayAdmissionRetryAfterSeconds = previousRetryAfter
	})

	initRelayAdmissionEnv()

	assert.Zero(t, RelayMaxConcurrentRequests)
	assert.Zero(t, RelayMaxActiveBodyBytes)
	assert.Zero(t, RelayMemoryBreakerHighPercent)
	assert.Equal(t, 75, RelayMemoryBreakerLowPercent)
	assert.Zero(t, RelayMemoryBreakerMaxTripSeconds)
	assert.Equal(t, 5, RelayAdmissionRetryAfterSeconds)
}

func TestInitRelayAdmissionEnvReadsValidatedMaxTripSeconds(t *testing.T) {
	previousMaxTripSeconds := RelayMemoryBreakerMaxTripSeconds
	t.Cleanup(func() {
		RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
	})

	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "positive duration", value: "300", want: 300},
		{name: "zero disables", value: "0", want: 0},
		{name: "negative duration is disabled", value: "-1", want: 0},
		{name: "invalid duration is disabled", value: "invalid", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("RELAY_MEMORY_BREAKER_MAX_TRIP_SECONDS", test.value)

			initRelayAdmissionEnv()

			assert.Equal(t, test.want, RelayMemoryBreakerMaxTripSeconds)
		})
	}
}

func TestInitRelayAdmissionEnvNormalizesInvalidHysteresis(t *testing.T) {
	t.Setenv("RELAY_MEMORY_BREAKER_HIGH_PERCENT", "80")
	t.Setenv("RELAY_MEMORY_BREAKER_LOW_PERCENT", "90")

	previousHighPercent := RelayMemoryBreakerHighPercent
	previousLowPercent := RelayMemoryBreakerLowPercent
	t.Cleanup(func() {
		RelayMemoryBreakerHighPercent = previousHighPercent
		RelayMemoryBreakerLowPercent = previousLowPercent
	})

	initRelayAdmissionEnv()

	assert.Equal(t, 80, RelayMemoryBreakerHighPercent)
	assert.Equal(t, 70, RelayMemoryBreakerLowPercent)
}
