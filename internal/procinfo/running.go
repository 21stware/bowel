package procinfo

// Running lists the programs running for this user, by their executables'
// paths. Another user's processes and those it can't read are left out, and
// on Windows the system's own (under %SystemRoot%); nil where no list can be
// had.
func Running() []string { return running() }
