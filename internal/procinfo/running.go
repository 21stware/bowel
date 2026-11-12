package procinfo

// Running lists the programs running for this user: their executables'
// paths, or on Windows their file names, as the core names them when it
// matches a rule for an application. Another user's processes and those it
// can't read are left out; nil where no list can be had.
func Running() []string { return running() }
