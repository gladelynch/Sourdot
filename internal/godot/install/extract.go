package install

// Zip extraction (Godot ships .zip on every platform, including macOS .app
// bundles -- no tar/gz handling needed). Extracts to a temp staging dir,
// verifies against Godot's published SHA512SUMS.txt, then atomically
// renames into place so a killed process never leaves a half-extracted
// install visible. Implemented in M1.
