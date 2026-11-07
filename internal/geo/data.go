package geo

// Data tables. Aliases written in ASCII match whole tokens case-insensitively
// (multi-word aliases match consecutive tokens, or the words written
// together); any other alias matches as a substring. A country's ISO code is
// always an alias of it. Coordinates are decimal degrees of the city centre.

type countryRow struct {
	code    string   // ISO 3166-1 alpha-2
	name    string   // display name in Simplified Chinese
	main    string   // display name of its main city, a row in cityRows
	aliases []string // besides the ISO code
}

type cityRow struct {
	country string // ISO 3166-1 alpha-2
	name    string // display name in Simplified Chinese
	lat     float64
	lon     float64
	aliases []string
}

var countryRows = []countryRow{
	// East Asia
	{"HK", "香港", "香港", []string{"香港", "中国香港", "中國香港", "Hong Kong", "HKG"}},
	{"MO", "澳门", "澳门", []string{"澳门", "澳門", "中国澳门", "中國澳門", "Macau", "Macao"}},
	{"TW", "台湾", "台北", []string{"台湾", "台灣", "臺灣", "中国台湾", "中國台灣", "Taiwan", "TWN"}},
	{"JP", "日本", "东京", []string{"日本", "Japan", "JPN"}},
	{"KR", "韩国", "首尔", []string{"韩国", "韓國", "南韩", "南韓", "Korea", "South Korea", "KOR"}},
	{"MN", "蒙古", "乌兰巴托", []string{"蒙古", "Mongolia"}},
	{"CN", "中国", "北京", []string{"中国", "中國", "大陆", "内蒙古", "China"}},

	// Southeast and South Asia
	{"SG", "新加坡", "新加坡", []string{"新加坡", "星加坡", "Singapore", "SGP"}},
	{"MY", "马来西亚", "吉隆坡", []string{"马来西亚", "馬來西亞", "大马", "Malaysia"}},
	{"TH", "泰国", "曼谷", []string{"泰国", "泰國", "Thailand"}},
	{"VN", "越南", "胡志明", []string{"越南", "Vietnam", "Viet Nam"}},
	{"PH", "菲律宾", "马尼拉", []string{"菲律宾", "菲律賓", "Philippines"}},
	{"ID", "印度尼西亚", "雅加达", []string{"印度尼西亚", "印尼", "Indonesia"}},
	{"KH", "柬埔寨", "金边", []string{"柬埔寨", "Cambodia"}},
	{"MM", "缅甸", "仰光", []string{"缅甸", "Myanmar"}},
	{"IN", "印度", "孟买", []string{"印度", "India"}},
	{"PK", "巴基斯坦", "卡拉奇", []string{"巴基斯坦", "Pakistan"}},
	{"BD", "孟加拉", "达卡", []string{"孟加拉", "Bangladesh"}},
	{"KZ", "哈萨克斯坦", "阿拉木图", []string{"哈萨克斯坦", "Kazakhstan"}},

	// Oceania
	{"AU", "澳大利亚", "悉尼", []string{"澳大利亚", "澳洲", "Australia"}},
	{"NZ", "新西兰", "奥克兰", []string{"新西兰", "紐西蘭", "New Zealand"}},

	// North America
	{"US", "美国", "洛杉矶", []string{"美国", "美國", "United States", "America", "USA"}},
	{"CA", "加拿大", "多伦多", []string{"加拿大", "Canada"}},
	{"MX", "墨西哥", "墨西哥城", []string{"墨西哥", "Mexico"}},

	// South America
	{"BR", "巴西", "圣保罗", []string{"巴西", "Brazil", "Brasil"}},
	{"AR", "阿根廷", "布宜诺斯艾利斯", []string{"阿根廷", "Argentina"}},
	{"CL", "智利", "圣地亚哥", []string{"智利", "Chile"}},
	{"CO", "哥伦比亚", "波哥大", []string{"哥伦比亚", "Colombia"}},
	{"PE", "秘鲁", "利马", []string{"秘鲁", "Peru"}},

	// Europe
	{"GB", "英国", "伦敦", []string{"英国", "英國", "United Kingdom", "Great Britain", "Britain", "England", "UK"}},
	{"DE", "德国", "法兰克福", []string{"德国", "德國", "Germany"}},
	{"FR", "法国", "巴黎", []string{"法国", "法國", "France"}},
	{"NL", "荷兰", "阿姆斯特丹", []string{"荷兰", "荷蘭", "Netherlands", "Holland"}},
	{"BE", "比利时", "布鲁塞尔", []string{"比利时", "Belgium"}},
	{"IE", "爱尔兰", "都柏林", []string{"爱尔兰", "Ireland"}},
	{"CH", "瑞士", "苏黎世", []string{"瑞士", "Switzerland"}},
	{"AT", "奥地利", "维也纳", []string{"奥地利", "Austria"}},
	{"IT", "意大利", "米兰", []string{"意大利", "Italy"}},
	{"ES", "西班牙", "马德里", []string{"西班牙", "Spain"}},
	{"PT", "葡萄牙", "里斯本", []string{"葡萄牙", "Portugal"}},
	{"SE", "瑞典", "斯德哥尔摩", []string{"瑞典", "Sweden"}},
	{"NO", "挪威", "奥斯陆", []string{"挪威", "Norway"}},
	{"DK", "丹麦", "哥本哈根", []string{"丹麦", "Denmark"}},
	{"FI", "芬兰", "赫尔辛基", []string{"芬兰", "Finland"}},
	{"PL", "波兰", "华沙", []string{"波兰", "Poland"}},
	{"CZ", "捷克", "布拉格", []string{"捷克", "Czech", "Czechia"}},
	{"HU", "匈牙利", "布达佩斯", []string{"匈牙利", "Hungary"}},
	{"RO", "罗马尼亚", "布加勒斯特", []string{"罗马尼亚", "Romania"}},
	{"UA", "乌克兰", "基辅", []string{"乌克兰", "Ukraine"}},
	{"RU", "俄罗斯", "莫斯科", []string{"俄罗斯", "俄羅斯", "俄国", "Russia"}},
	{"TR", "土耳其", "伊斯坦布尔", []string{"土耳其", "Turkey", "Turkiye"}},

	// Middle East and Africa
	{"IL", "以色列", "特拉维夫", []string{"以色列", "Israel"}},
	{"AE", "阿联酋", "迪拜", []string{"阿联酋", "阿拉伯联合酋长国", "United Arab Emirates", "Emirates", "UAE"}},
	{"SA", "沙特阿拉伯", "利雅得", []string{"沙特阿拉伯", "沙特", "Saudi Arabia", "Saudi"}},
	{"QA", "卡塔尔", "多哈", []string{"卡塔尔", "Qatar"}},
	{"EG", "埃及", "开罗", []string{"埃及", "Egypt"}},
	{"ZA", "南非", "约翰内斯堡", []string{"南非", "South Africa"}},
	{"NG", "尼日利亚", "拉各斯", []string{"尼日利亚", "Nigeria"}},
	{"KE", "肯尼亚", "内罗毕", []string{"肯尼亚", "Kenya"}},
}

// cityRows lists cities by country. IATA codes among the aliases are the
// ones providers write in names; ambiguous ones (SEA, LAS, MEL, ORD, ATL,
// MIA, HAN, …) are left out here but still appear in coloRows.
var cityRows = []cityRow{
	{"HK", "香港", 22.32, 114.17, []string{"香港", "Hong Kong"}},
	{"MO", "澳门", 22.20, 113.54, []string{"澳门", "澳門", "Macau", "Macao", "MFM"}},

	{"JP", "东京", 35.68, 139.69, []string{"东京", "東京", "Tokyo", "NRT", "HND"}},
	{"JP", "大阪", 34.69, 135.50, []string{"大阪", "Osaka", "KIX"}},
	{"JP", "埼玉", 35.86, 139.65, []string{"埼玉", "Saitama"}},
	{"JP", "福冈", 33.59, 130.40, []string{"福冈", "福岡", "Fukuoka", "FUK"}},

	{"TW", "台北", 25.03, 121.57, []string{"台北", "臺北", "Taipei", "TPE"}},
	{"TW", "新北", 25.01, 121.47, []string{"新北", "New Taipei"}},
	{"TW", "彰化", 24.08, 120.54, []string{"彰化", "Changhua"}},
	{"TW", "台中", 24.15, 120.67, []string{"台中", "臺中", "Taichung"}},
	{"TW", "高雄", 22.63, 120.30, []string{"高雄", "Kaohsiung", "KHH"}},

	{"KR", "首尔", 37.57, 126.98, []string{"首尔", "首爾", "汉城", "Seoul", "ICN"}},
	{"KR", "春川", 37.88, 127.73, []string{"春川", "Chuncheon"}},

	{"MN", "乌兰巴托", 47.89, 106.91, []string{"乌兰巴托", "Ulaanbaatar"}},

	// Mainland cities appear mostly as relay entry points; see weakCountry.
	{"CN", "北京", 39.90, 116.40, []string{"北京", "Beijing"}},
	{"CN", "上海", 31.23, 121.47, []string{"上海", "Shanghai"}},
	{"CN", "广州", 23.13, 113.26, []string{"广州", "廣州", "Guangzhou"}},
	{"CN", "深圳", 22.54, 114.06, []string{"深圳", "Shenzhen"}},

	{"SG", "新加坡", 1.35, 103.82, []string{"新加坡", "狮城", "獅城", "Singapore", "SIN"}},
	{"MY", "吉隆坡", 3.14, 101.69, []string{"吉隆坡", "Kuala Lumpur", "KUL"}},
	{"TH", "曼谷", 13.76, 100.50, []string{"曼谷", "Bangkok", "BKK"}},
	{"VN", "河内", 21.03, 105.85, []string{"河内", "河內", "Hanoi", "Ha Noi"}},
	{"VN", "胡志明", 10.82, 106.63, []string{"胡志明", "西贡", "Ho Chi Minh", "Saigon", "HCMC", "SGN"}},
	{"PH", "马尼拉", 14.60, 120.98, []string{"马尼拉", "馬尼拉", "Manila", "MNL"}},
	{"ID", "雅加达", -6.21, 106.85, []string{"雅加达", "Jakarta", "CGK"}},
	{"KH", "金边", 11.56, 104.92, []string{"金边", "Phnom Penh"}},
	{"MM", "仰光", 16.84, 96.17, []string{"仰光", "Yangon"}},
	{"IN", "孟买", 19.08, 72.88, []string{"孟买", "Mumbai", "Bombay", "BOM"}},
	{"IN", "新德里", 28.61, 77.21, []string{"新德里", "德里", "New Delhi", "Delhi"}},
	{"PK", "卡拉奇", 24.86, 67.01, []string{"卡拉奇", "Karachi"}},
	{"BD", "达卡", 23.81, 90.41, []string{"达卡", "Dhaka"}},
	{"KZ", "阿拉木图", 43.24, 76.89, []string{"阿拉木图", "Almaty"}},

	{"AU", "悉尼", -33.87, 151.21, []string{"悉尼", "雪梨", "Sydney", "SYD"}},
	{"AU", "墨尔本", -37.81, 144.96, []string{"墨尔本", "Melbourne"}},
	{"AU", "布里斯班", -27.47, 153.03, []string{"布里斯班", "Brisbane"}},
	{"AU", "珀斯", -31.95, 115.86, []string{"珀斯", "Perth"}},
	// "奥克兰" is also Oakland, California, so only the English name matches.
	{"NZ", "奥克兰", -36.85, 174.76, []string{"Auckland"}},

	{"US", "洛杉矶", 34.05, -118.24, []string{"洛杉矶", "洛杉磯", "Los Angeles", "LAX"}},
	{"US", "圣何塞", 37.34, -121.89, []string{"圣何塞", "聖何塞", "圣荷西", "San Jose", "SJC"}},
	{"US", "哥伦布", 39.96, -83.00, []string{"哥伦布", "Columbus"}},
	{"US", "明尼阿波利斯", 44.98, -93.27, []string{"明尼阿波利斯", "Minneapolis"}},
	{"US", "费城", 39.95, -75.17, []string{"费城", "Philadelphia"}},
	{"US", "堪萨斯城", 39.10, -94.58, []string{"堪萨斯城", "Kansas City"}},
	{"US", "盐湖城", 40.76, -111.89, []string{"盐湖城", "Salt Lake City"}},
	{"US", "夏洛特", 35.23, -80.84, []string{"夏洛特", "Charlotte"}},
	{"US", "纳什维尔", 36.16, -86.78, []string{"纳什维尔", "Nashville"}},
	{"US", "硅谷", 37.39, -122.03, []string{"硅谷", "矽谷", "Silicon Valley"}},
	{"US", "旧金山", 37.77, -122.42, []string{"旧金山", "舊金山", "三藩市", "San Francisco", "SFO"}},
	{"US", "西雅图", 47.61, -122.33, []string{"西雅图", "西雅圖", "Seattle"}},
	{"US", "波特兰", 45.52, -122.68, []string{"波特兰", "Portland", "PDX"}},
	{"US", "凤凰城", 33.45, -112.07, []string{"凤凰城", "菲尼克斯", "Phoenix", "PHX"}},
	{"US", "拉斯维加斯", 36.17, -115.14, []string{"拉斯维加斯", "赌城", "Las Vegas"}},
	{"US", "达拉斯", 32.78, -96.80, []string{"达拉斯", "Dallas", "DFW"}},
	{"US", "休斯顿", 29.76, -95.37, []string{"休斯顿", "Houston"}},
	{"US", "丹佛", 39.74, -104.99, []string{"丹佛", "Denver"}},
	{"US", "芝加哥", 41.88, -87.63, []string{"芝加哥", "Chicago"}},
	{"US", "纽约", 40.71, -74.01, []string{"纽约", "紐約", "New York", "NYC", "JFK", "EWR"}},
	{"US", "波士顿", 42.36, -71.06, []string{"波士顿", "Boston"}},
	{"US", "华盛顿", 38.91, -77.04, []string{"华盛顿", "華盛頓", "Washington"}},
	{"US", "阿什本", 39.04, -77.49, []string{"阿什本", "Ashburn", "IAD"}},
	{"US", "亚特兰大", 33.75, -84.39, []string{"亚特兰大", "Atlanta"}},
	{"US", "迈阿密", 25.76, -80.19, []string{"迈阿密", "Miami"}},

	{"CA", "多伦多", 43.65, -79.38, []string{"多伦多", "Toronto", "YYZ"}},
	{"CA", "温哥华", 49.28, -123.12, []string{"温哥华", "溫哥華", "Vancouver", "YVR"}},
	{"CA", "蒙特利尔", 45.50, -73.57, []string{"蒙特利尔", "满地可", "Montreal", "YUL"}},
	{"MX", "墨西哥城", 19.43, -99.13, []string{"墨西哥城", "Mexico City"}},

	{"BR", "圣保罗", -23.55, -46.63, []string{"圣保罗", "Sao Paulo", "GRU"}},
	{"AR", "布宜诺斯艾利斯", -34.60, -58.38, []string{"布宜诺斯艾利斯", "Buenos Aires"}},
	// "圣地亚哥" is also San Diego, California, so only the English name matches.
	{"CL", "圣地亚哥", -33.45, -70.67, []string{"Santiago"}},
	{"CO", "波哥大", 4.71, -74.07, []string{"波哥大", "Bogota"}},
	{"PE", "利马", -12.05, -77.04, []string{"利马", "Lima"}},

	{"GB", "伦敦", 51.51, -0.13, []string{"伦敦", "倫敦", "London", "LHR"}},
	{"GB", "曼彻斯特", 53.48, -2.24, []string{"曼彻斯特", "Manchester"}},
	{"DE", "法兰克福", 50.11, 8.68, []string{"法兰克福", "法蘭克福", "Frankfurt", "FRA"}},
	{"DE", "柏林", 52.52, 13.40, []string{"柏林", "Berlin"}},
	{"DE", "慕尼黑", 48.14, 11.58, []string{"慕尼黑", "Munich"}},
	{"FR", "巴黎", 48.86, 2.35, []string{"巴黎", "Paris", "CDG"}},
	{"FR", "马赛", 43.30, 5.37, []string{"马赛", "Marseille"}},
	{"NL", "阿姆斯特丹", 52.37, 4.90, []string{"阿姆斯特丹", "Amsterdam", "AMS"}},
	{"BE", "布鲁塞尔", 50.85, 4.35, []string{"布鲁塞尔", "Brussels"}},
	{"IE", "都柏林", 53.35, -6.26, []string{"都柏林", "Dublin"}},
	{"CH", "苏黎世", 47.37, 8.54, []string{"苏黎世", "Zurich"}},
	{"AT", "维也纳", 48.21, 16.37, []string{"维也纳", "Vienna"}},
	{"IT", "米兰", 45.46, 9.19, []string{"米兰", "Milan"}},
	{"IT", "罗马", 41.90, 12.50, []string{"罗马", "Rome"}},
	{"ES", "马德里", 40.42, -3.70, []string{"马德里", "Madrid"}},
	{"PT", "里斯本", 38.72, -9.14, []string{"里斯本", "Lisbon"}},
	{"SE", "斯德哥尔摩", 59.33, 18.07, []string{"斯德哥尔摩", "Stockholm"}},
	{"NO", "奥斯陆", 59.91, 10.75, []string{"奥斯陆", "Oslo"}},
	{"DK", "哥本哈根", 55.68, 12.57, []string{"哥本哈根", "Copenhagen"}},
	{"FI", "赫尔辛基", 60.17, 24.94, []string{"赫尔辛基", "Helsinki"}},
	{"PL", "华沙", 52.23, 21.01, []string{"华沙", "Warsaw"}},
	{"CZ", "布拉格", 50.08, 14.44, []string{"布拉格", "Prague"}},
	{"HU", "布达佩斯", 47.50, 19.04, []string{"布达佩斯", "Budapest"}},
	{"RO", "布加勒斯特", 44.43, 26.10, []string{"布加勒斯特", "Bucharest"}},
	{"UA", "基辅", 50.45, 30.52, []string{"基辅", "Kyiv", "Kiev"}},
	{"RU", "莫斯科", 55.76, 37.62, []string{"莫斯科", "Moscow"}},
	{"RU", "圣彼得堡", 59.93, 30.34, []string{"圣彼得堡", "Saint Petersburg", "St Petersburg"}},
	{"TR", "伊斯坦布尔", 41.01, 28.98, []string{"伊斯坦布尔", "Istanbul", "IST"}},

	{"IL", "特拉维夫", 32.09, 34.78, []string{"特拉维夫", "Tel Aviv"}},
	{"AE", "迪拜", 25.20, 55.27, []string{"迪拜", "杜拜", "Dubai", "DXB"}},
	{"SA", "利雅得", 24.71, 46.68, []string{"利雅得", "Riyadh"}},
	{"QA", "多哈", 25.29, 51.53, []string{"多哈", "Doha"}},
	{"EG", "开罗", 30.04, 31.24, []string{"开罗", "Cairo"}},
	{"ZA", "约翰内斯堡", -26.20, 28.05, []string{"约翰内斯堡", "Johannesburg"}},
	{"NG", "拉各斯", 6.52, 3.38, []string{"拉各斯", "Lagos"}},
	{"KE", "内罗毕", -1.29, 36.82, []string{"内罗毕", "Nairobi"}},
}

// coloRows maps Cloudflare data-centre IATA codes to a city in cityRows.
var coloRows = map[string]string{
	// East Asia
	"HKG": "香港", "MFM": "澳门",
	"NRT": "东京", "HND": "东京", "KIX": "大阪", "FUK": "福冈",
	"TPE": "台北", "KHH": "高雄",
	"ICN": "首尔",
	"ULN": "乌兰巴托",
	"PEK": "北京", "PVG": "上海", "CAN": "广州", "SZX": "深圳",

	// Southeast and South Asia
	"SIN": "新加坡", "KUL": "吉隆坡", "BKK": "曼谷",
	"HAN": "河内", "SGN": "胡志明", "MNL": "马尼拉", "CGK": "雅加达",
	"PNH": "金边", "RGN": "仰光",
	"BOM": "孟买", "DEL": "新德里", "KHI": "卡拉奇", "DAC": "达卡", "ALA": "阿拉木图",

	// Oceania
	"SYD": "悉尼", "MEL": "墨尔本", "BNE": "布里斯班", "PER": "珀斯", "AKL": "奥克兰",

	// North America
	"LAX": "洛杉矶", "SJC": "圣何塞", "SFO": "旧金山", "SEA": "西雅图", "PDX": "波特兰",
	"PHX": "凤凰城", "LAS": "拉斯维加斯", "DFW": "达拉斯", "IAH": "休斯顿", "DEN": "丹佛",
	"ORD": "芝加哥", "EWR": "纽约", "JFK": "纽约", "BOS": "波士顿", "IAD": "阿什本",
	"ATL": "亚特兰大", "MIA": "迈阿密", "CMH": "哥伦布", "MSP": "明尼阿波利斯", "PHL": "费城",
	"MCI": "堪萨斯城", "SLC": "盐湖城", "CLT": "夏洛特", "BNA": "纳什维尔",
	"YYZ": "多伦多", "YVR": "温哥华", "YUL": "蒙特利尔", "MEX": "墨西哥城",

	// South America
	"GRU": "圣保罗", "EZE": "布宜诺斯艾利斯", "SCL": "圣地亚哥", "BOG": "波哥大", "LIM": "利马",

	// Europe
	"LHR": "伦敦", "MAN": "曼彻斯特", "FRA": "法兰克福", "BER": "柏林", "MUC": "慕尼黑",
	"CDG": "巴黎", "MRS": "马赛", "AMS": "阿姆斯特丹", "BRU": "布鲁塞尔", "DUB": "都柏林",
	"ZRH": "苏黎世", "VIE": "维也纳", "MXP": "米兰", "FCO": "罗马", "MAD": "马德里",
	"LIS": "里斯本", "ARN": "斯德哥尔摩", "OSL": "奥斯陆", "CPH": "哥本哈根", "HEL": "赫尔辛基",
	"WAW": "华沙", "PRG": "布拉格", "BUD": "布达佩斯", "OTP": "布加勒斯特", "KBP": "基辅",
	"DME": "莫斯科", "SVO": "莫斯科", "LED": "圣彼得堡", "IST": "伊斯坦布尔",

	// Middle East and Africa
	"TLV": "特拉维夫", "DXB": "迪拜", "RUH": "利雅得", "DOH": "多哈",
	"CAI": "开罗", "JNB": "约翰内斯堡", "LOS": "拉各斯", "NBO": "内罗毕",
}
