package project

// Pin-file read/resolve. Own format is .godotvm-version, a plain
// single-line file (e.g. "4.2.1-mono") with no OS/arch baked in so a pin
// checked into git stays portable. Read-compat (read-only, never written)
// for migration from incumbents: .godot-version (same convention) and
// .tool-versions (parse the "godot " line, ignore other tools).
//
// Resolution precedence at launch: explicit UI pin (store.BucketProjects)
// > .godotvm-version > .godot-version > .tool-versions > best-effort
// project.godot config_version detection > prompt the user.
//
// Implemented in M3.
