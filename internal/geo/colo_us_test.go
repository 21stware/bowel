package geo

import "testing"

func TestUSColos(t *testing.T) {
	for colo, city := range map[string]string{"CMH": "哥伦布", "MSP": "明尼阿波利斯", "PHL": "费城", "SLC": "盐湖城"} {
		if p, ok := FromTrace("US", colo); !ok || p.City != city {
			t.Errorf("%s = %+v %v", colo, p, ok)
		}
	}
}
