package main

// The status in a container. Takeover there is the system proxy, which a
// container refuses, or TUN, which is usually off, so takeover is "off"
// nearly always. Yet the ports are how traffic comes in, takeover or not:
// a core that fails, a proxy port that can't open and a locked group that
// refuses every connection matter there as they do under a takeover that
// runs. So the light, the issue and a switch's news judge a container as
// that takeover; the takeover's own entries (the system proxy, TUN, the
// lock, the gateway) still say what each of them is.

// judgedOn says whether the status is judged as a running takeover's:
// when takeover is on, and in a container always.
func judgedOn(on bool) bool { return on || inContainer() }
