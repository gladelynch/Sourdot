package project

// Minimal hand-rolled parser for project.godot -- not a generic INI library,
// since only a handful of keys matter: [application] config/name,
// config_version (4 = Godot 3.x, 5 = Godot 4.x -- more reliable than
// parsing the config/features PackedStringArray), and a [dotnet] section
// (or sibling .csproj) for C# detection. Implemented in M3.
