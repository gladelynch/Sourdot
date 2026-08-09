package release

// matchAsset will hold the (major, isMono, OS, arch) -> regex pattern table
// described in the plan, used to pick the correct Asset out of a Release for
// the current platform. Godot's asset naming/layout differs meaningfully
// between 3.x and 4.x (3.x mono ships a folder, not a flat zip) and needs
// real fixture data (internal/godot/release/testdata/) to get right --
// implemented in M1, not here.
