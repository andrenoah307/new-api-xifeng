package geoip

import "strings"

var chineseToISO = map[string]string{
	"中国":    "CN",
	"美国":    "US",
	"日本":    "JP",
	"韩国":    "KR",
	"英国":    "GB",
	"法国":    "FR",
	"德国":    "DE",
	"俄罗斯":   "RU",
	"加拿大":   "CA",
	"澳大利亚":  "AU",
	"印度":    "IN",
	"巴西":    "BR",
	"新加坡":   "SG",
	"中国香港":  "HK",
	"中国台湾":  "TW",
	"中国澳门":  "MO",
	"荷兰":    "NL",
	"瑞典":    "SE",
	"瑞士":    "CH",
	"意大利":   "IT",
	"西班牙":   "ES",
	"葡萄牙":   "PT",
	"墨西哥":   "MX",
	"阿根廷":   "AR",
	"智利":    "CL",
	"哥伦比亚":  "CO",
	"秘鲁":    "PE",
	"印度尼西亚": "ID",
	"马来西亚":  "MY",
	"泰国":    "TH",
	"越南":    "VN",
	"菲律宾":   "PH",
	"土耳其":   "TR",
	"以色列":   "IL",
	"沙特阿拉伯": "SA",
	"阿联酋":   "AE",
	"伊朗":    "IR",
	"伊拉克":   "IQ",
	"波兰":    "PL",
	"乌克兰":   "UA",
	"埃及":    "EG",
	"南非":    "ZA",
	"尼日利亚":  "NG",
	"肯尼亚":   "KE",
	"新西兰":   "NZ",
	"芬兰":    "FI",
	"挪威":    "NO",
	"丹麦":    "DK",
	"爱尔兰":   "IE",
	"奥地利":   "AT",
	"比利时":   "BE",
	"捷克":    "CZ",
	"罗马尼亚":  "RO",
	"匈牙利":   "HU",
	"希腊":    "GR",
	"孟加拉":   "BD",
	"巴基斯坦":  "PK",
	"缅甸":    "MM",
	"柬埔寨":   "KH",
	"老挝":    "LA",
}

func ChineseNameToCountryCode(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "0" {
		return ""
	}
	if code, ok := chineseToISO[name]; ok {
		return code
	}
	return strings.ToUpper(name)
}
