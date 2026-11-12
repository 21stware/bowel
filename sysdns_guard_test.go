package main

// No test may set this machine's DNS servers: the ones about the takeover
// put their own fake in osDNS's place.
func init() { osDNS.supported = false }
