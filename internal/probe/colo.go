package probe

// Datacenter and country coordinates for the map pins.
//
// This table is deliberately partial. Cloudflare operates in 300-plus
// cities and the list moves; a demo app does not need to track it. An
// unknown code is not an error — ColoKnown stays false, the map skips
// that pin, and the rest of the UI is unaffected.

// coloSite is one Cloudflare datacenter location.
type coloSite struct {
	City string
	Lat  float64
	Lng  float64
}

// coloSites maps the three-letter IATA-style code the trace endpoint
// reports to a city and its coordinates.
var coloSites = map[string]coloSite{
	// North America
	"ATL": {"Atlanta", 33.6407, -84.4277},
	"BOS": {"Boston", 42.3656, -71.0096},
	"DEN": {"Denver", 39.8561, -104.6737},
	"DFW": {"Dallas", 32.8998, -97.0403},
	"EWR": {"Newark", 40.6895, -74.1745},
	"IAD": {"Ashburn", 38.9531, -77.4565},
	"LAX": {"Los Angeles", 33.9416, -118.4085},
	"MCI": {"Kansas City", 39.2976, -94.7139},
	"MIA": {"Miami", 25.7959, -80.2871},
	"ORD": {"Chicago", 41.9742, -87.9073},
	"PDX": {"Portland", 45.5898, -122.5951},
	"PHX": {"Phoenix", 33.4342, -112.0116},
	"SEA": {"Seattle", 47.4502, -122.3088},
	"SJC": {"San Jose", 37.3639, -121.9289},
	"YUL": {"Montreal", 45.4706, -73.7408},
	"YVR": {"Vancouver", 49.1967, -123.1815},
	"YYZ": {"Toronto", 43.6777, -79.6248},
	"MEX": {"Mexico City", 19.4363, -99.0721},
	"ABQ": {"Albuquerque", 35.0402, -106.6091},
	"ANC": {"Anchorage", 61.1743, -149.9962},
	"AUS": {"Austin", 30.1975, -97.6664},
	"BNA": {"Nashville", 36.1263, -86.6774},
	"BUF": {"Buffalo", 42.9405, -78.7322},
	"CLE": {"Cleveland", 41.4117, -81.8498},
	"CMH": {"Columbus", 39.9980, -82.8919},
	"DTW": {"Detroit", 42.2124, -83.3534},
	"HNL": {"Honolulu", 21.3187, -157.9225},
	"IAH": {"Houston", 29.9902, -95.3368},
	"IND": {"Indianapolis", 39.7173, -86.2944},
	"LAS": {"Las Vegas", 36.0840, -115.1537},
	"MSP": {"Minneapolis", 44.8848, -93.2223},
	"OMA": {"Omaha", 41.3032, -95.8941},
	"PIT": {"Pittsburgh", 40.4915, -80.2329},
	"SAN": {"San Diego", 32.7336, -117.1897},
	"SLC": {"Salt Lake City", 40.7899, -111.9791},
	"SMF": {"Sacramento", 38.6951, -121.5908},
	"STL": {"St. Louis", 38.7487, -90.3700},
	"TPA": {"Tampa", 27.9755, -82.5332},

	// South America
	"EZE": {"Buenos Aires", -34.8222, -58.5358},
	"GIG": {"Rio de Janeiro", -22.8100, -43.2506},
	"GRU": {"Sao Paulo", -23.4356, -46.4731},
	"SCL": {"Santiago", -33.3930, -70.7858},
	"BOG": {"Bogota", 4.7016, -74.1469},
	"LIM": {"Lima", -12.0219, -77.1143},

	// Europe
	"AMS": {"Amsterdam", 52.3105, 4.7683},
	"ARN": {"Stockholm", 59.6519, 17.9186},
	"BCN": {"Barcelona", 41.2974, 2.0833},
	"BRU": {"Brussels", 50.9014, 4.4844},
	"BUD": {"Budapest", 47.4298, 19.2611},
	"CDG": {"Paris", 49.0097, 2.5479},
	"CPH": {"Copenhagen", 55.6180, 12.6508},
	"DUB": {"Dublin", 53.4213, -6.2701},
	"DUS": {"Dusseldorf", 51.2895, 6.7668},
	"FRA": {"Frankfurt", 50.0379, 8.5622},
	"HEL": {"Helsinki", 60.3172, 24.9633},
	"IST": {"Istanbul", 41.2753, 28.7519},
	"LHR": {"London", 51.4700, -0.4543},
	"LIS": {"Lisbon", 38.7742, -9.1342},
	"MAD": {"Madrid", 40.4936, -3.5668},
	"MAN": {"Manchester", 53.3654, -2.2725},
	"MRS": {"Marseille", 43.4393, 5.2214},
	"MUC": {"Munich", 48.3537, 11.7750},
	"MXP": {"Milan", 45.6301, 8.7255},
	"OSL": {"Oslo", 60.1939, 11.1004},
	"OTP": {"Bucharest", 44.5711, 26.0850},
	"PRG": {"Prague", 50.1008, 14.2600},
	"VIE": {"Vienna", 48.1103, 16.5697},
	"WAW": {"Warsaw", 52.1657, 20.9671},
	"ZRH": {"Zurich", 47.4647, 8.5492},

	// Asia and Pacific
	"BLR": {"Bangalore", 13.1986, 77.7066},
	"BOM": {"Mumbai", 19.0896, 72.8656},
	"DEL": {"Delhi", 28.5562, 77.1000},
	"HKG": {"Hong Kong", 22.3080, 113.9185},
	"ICN": {"Seoul", 37.4602, 126.4407},
	"KIX": {"Osaka", 34.4273, 135.2440},
	"KUL": {"Kuala Lumpur", 2.7456, 101.7072},
	"MNL": {"Manila", 14.5086, 121.0198},
	"NRT": {"Tokyo", 35.7720, 140.3929},
	"SIN": {"Singapore", 1.3644, 103.9915},
	"TPE": {"Taipei", 25.0777, 121.2328},
	"AKL": {"Auckland", -37.0082, 174.7850},
	"BNE": {"Brisbane", -27.3842, 153.1175},
	"MEL": {"Melbourne", -37.6690, 144.8410},
	"PER": {"Perth", -31.9385, 115.9672},
	"SYD": {"Sydney", -33.9399, 151.1753},

	// Middle East and Africa
	"CAI": {"Cairo", 30.1219, 31.4056},
	"CPT": {"Cape Town", -33.9649, 18.6017},
	"DOH": {"Doha", 25.2731, 51.6081},
	"DXB": {"Dubai", 25.2532, 55.3657},
	"JNB": {"Johannesburg", -26.1392, 28.2460},
	"LOS": {"Lagos", 6.5774, 3.3212},
	"NBO": {"Nairobi", -1.3192, 36.9278},
	"TLV": {"Tel Aviv", 32.0114, 34.8867},
}

// countryCentroids maps the two-letter country code the trace endpoint
// reports to a rough centre point. The client pin is only ever this
// coarse: the app does no geo-IP lookup and sends the address nowhere.
var countryCentroids = map[string][2]float64{
	"AE": {23.42, 53.85}, "AR": {-38.42, -63.62}, "AT": {47.52, 14.55},
	"AU": {-25.27, 133.78}, "BE": {50.50, 4.47}, "BG": {42.73, 25.49},
	"BR": {-14.24, -51.93}, "CA": {56.13, -106.35}, "CH": {46.82, 8.23},
	"CL": {-35.68, -71.54}, "CN": {35.86, 104.20}, "CO": {4.57, -74.30},
	"CZ": {49.82, 15.47}, "DE": {51.17, 10.45}, "DK": {56.26, 9.50},
	"EG": {26.82, 30.80}, "ES": {40.46, -3.75}, "FI": {61.92, 25.75},
	"FR": {46.23, 2.21}, "GB": {55.38, -3.44}, "GR": {39.07, 21.82},
	"HK": {22.32, 114.17}, "HU": {47.16, 19.50}, "ID": {-0.79, 113.92},
	"IE": {53.41, -8.24}, "IL": {31.05, 34.85}, "IN": {20.59, 78.96},
	"IT": {41.87, 12.57}, "JP": {36.20, 138.25}, "KE": {-0.02, 37.91},
	"KR": {35.91, 127.77}, "MX": {23.63, -102.55}, "MY": {4.21, 101.98},
	"NG": {9.08, 8.68}, "NL": {52.13, 5.29}, "NO": {60.47, 8.47},
	"NZ": {-40.90, 174.89}, "PE": {-9.19, -75.02}, "PH": {12.88, 121.77},
	"PL": {51.92, 19.15}, "PT": {39.40, -8.22}, "RO": {45.94, 24.97},
	"SA": {23.89, 45.08}, "SE": {60.13, 18.64}, "SG": {1.35, 103.82},
	"TH": {15.87, 100.99}, "TR": {38.96, 35.24}, "TW": {23.70, 120.96},
	"UA": {48.38, 31.17}, "US": {37.09, -95.71}, "VN": {14.06, 108.28},
	"ZA": {-30.56, 22.94},
}

// resolveLocations fills the coordinate fields of t from its Colo and
// Loc codes. Unknown codes leave the matching Known flag false.
func resolveLocations(t *Trace) {
	if site, ok := coloSites[t.Colo]; ok {
		t.ColoCity = site.City
		t.ColoLat = site.Lat
		t.ColoLng = site.Lng
		t.ColoKnown = true
	}
	if c, ok := countryCentroids[t.Loc]; ok {
		t.ClientLat = c[0]
		t.ClientLng = c[1]
		t.ClientKnown = true
	}
}
