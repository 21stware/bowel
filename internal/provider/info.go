package provider

import "regexp"

// InfoPattern matches the names of the entries a subscription lists as
// nodes only to say something: the traffic left, when the plan ends, the
// provider's website ("剩余流量：915.41 GB", "套餐到期：长期有效",
// "官网：example.com", "Expire: 2026-12-31"). Their servers go nowhere,
// so an automatic group that picks one fails over and reports it, and they
// swell the node count. What they say comes in the subscription-userinfo
// header anyway (Usage).
//
// It is handed to the core as each Provider's exclude-filter, so these
// entries never become nodes, and bowel reads the files with it too
// (Names, Servers). The core compiles it with regexp2, so it keeps to what
// that and Go's regexp read alike, and has no backtick, which the core
// splits exclude-filter on. Subscriptions write in either script, so
// both are here. Real nodes are named for where they exit; the
// words here don't name a place.
const InfoPattern = `剩[余餘]流量|流量剩[余餘]|已用流量|[总總]流量|套餐到期|到期[时時][间間]|[过過]期[时時][间間]|有效期|下次重置|重置剩[余餘]|流量重置|官[网網]|官方[网網]站|最新[网網]址|[备備]用[网網]址|[发發][布佈][页頁]|(?i:traffic|expire|remaining)\s*[:：]`

var infoName = regexp.MustCompile(InfoPattern)

// IsInfo says whether a node name is one of those InfoPattern describes.
func IsInfo(name string) bool { return infoName.MatchString(name) }
