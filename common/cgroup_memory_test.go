package common

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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
			}, 80, 70)

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
			breaker.update(cgroupMemorySample{usageBytes: test.tripUsage, limitBytes: 100, available: true}, uint64(test.high), uint64(low))
			require.True(t, breaker.isTripped())

			breaker.update(cgroupMemorySample{usageBytes: test.recoveryUsage, limitBytes: 100, available: true}, uint64(test.high), uint64(low))
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
	sampler := &cgroupMemorySampler{
		cgroupFile:    fixture.cgroupFile,
		mountInfoFile: fixture.mountInfoFile,
		highPercent:   80,
		lowPercent:    70,
		breaker:       breaker,
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

func TestInitCgroupMemorySamplerIsOptInAndExposesAtomicState(t *testing.T) {
	previousHigh := RelayMemoryBreakerHighPercent
	previousLow := RelayMemoryBreakerLowPercent
	t.Cleanup(func() {
		RelayMemoryBreakerHighPercent = previousHigh
		RelayMemoryBreakerLowPercent = previousLow
	})

	RelayMemoryBreakerHighPercent = 0
	RelayMemoryBreakerLowPercent = 75
	InitCgroupMemorySampler()

	RelayMemoryBreakerHighPercent = 101
	RelayMemoryBreakerLowPercent = 100
	InitCgroupMemorySampler()
	assert.False(t, IsCgroupMemoryPressure())
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
		"RELAY_ADMISSION_RETRY_AFTER_SECONDS",
	} {
		t.Setenv(env, "")
	}

	previousMaxRequests := RelayMaxConcurrentRequests
	previousMaxBodyBytes := RelayMaxActiveBodyBytes
	previousHighPercent := RelayMemoryBreakerHighPercent
	previousLowPercent := RelayMemoryBreakerLowPercent
	previousRetryAfter := RelayAdmissionRetryAfterSeconds
	t.Cleanup(func() {
		RelayMaxConcurrentRequests = previousMaxRequests
		RelayMaxActiveBodyBytes = previousMaxBodyBytes
		RelayMemoryBreakerHighPercent = previousHighPercent
		RelayMemoryBreakerLowPercent = previousLowPercent
		RelayAdmissionRetryAfterSeconds = previousRetryAfter
	})

	initRelayAdmissionEnv()

	assert.Zero(t, RelayMaxConcurrentRequests)
	assert.Zero(t, RelayMaxActiveBodyBytes)
	assert.Zero(t, RelayMemoryBreakerHighPercent)
	assert.Equal(t, 75, RelayMemoryBreakerLowPercent)
	assert.Equal(t, 5, RelayAdmissionRetryAfterSeconds)
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
