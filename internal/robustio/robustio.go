// Package robustio replaces and removes files the mihomo core may have open:
// a provider's nodes, a rule set's rules. On Windows a file can't be
// replaced or removed while another program has it open, and the core opens
// these each time it loads them, or sees them change. There, a rename onto
// such a file fails with ERROR_ACCESS_DENIED and a remove with
// ERROR_SHARING_VIOLATION until the core lets go, which is soon, so both are
// tried again for a while, as cmd/go's robustio does. Elsewhere a file the
// core has open is replaced all the same, and the core goes on reading what
// it opened.
package robustio
