package geoip

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

var globalSearcher atomic.Pointer[xdb.Searcher]

func Init(xdbPath string) {
	if xdbPath == "" {
		xdbPath = "data/ip2region.xdb"
	}
	go func() {
		buf, err := os.ReadFile(xdbPath)
		if err != nil {
			common.SysLog("geoip: xdb file not found at " + xdbPath + ", region restriction disabled")
			return
		}
		header, err := xdb.NewHeader(buf)
		if err != nil {
			common.SysError("geoip: failed to parse xdb header: " + err.Error())
			return
		}
		version, err := xdb.VersionFromHeader(header)
		if err != nil {
			common.SysError("geoip: failed to detect xdb version: " + err.Error())
			return
		}
		s, err := xdb.NewWithBuffer(version, buf)
		if err != nil {
			common.SysError("geoip: failed to create searcher: " + err.Error())
			return
		}
		globalSearcher.Store(s)
		common.SysLog("geoip: ip2region loaded successfully from " + xdbPath)
	}()
}

func LookupCountry(ip string) string {
	s := globalSearcher.Load()
	if s == nil {
		return ""
	}
	region, err := s.Search(ip)
	if err != nil {
		return ""
	}
	return parseCountryFromRegion(region)
}

func IsReady() bool {
	return globalSearcher.Load() != nil
}

func parseCountryFromRegion(region string) string {
	parts := strings.SplitN(region, "|", 2)
	if len(parts) == 0 {
		return ""
	}
	return ChineseNameToCountryCode(parts[0])
}
