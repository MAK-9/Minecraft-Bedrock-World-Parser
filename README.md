# Minecraft Bedrock World Parser

An intermediate step in the world-to-map pipeline:

```
Bedrock world  →  minecraft-bedrock-parser  →  SQLite  →  dynamic map
```

Reads a Minecraft Bedrock Edition world from its native LevelDB format and writes the data into a SQLite database optimised for viewport-based loading by a dynamic map renderer.

---

## Requirements

- Go 1.22+
- A Minecraft Bedrock Edition world directory

## Build

```bash
git clone https://github.com/MAK-9/Minecraft-Bedrock-World-Parser
cd Minecraft-Bedrock-World-Parser
go build -o minecraft-bedrock-parser ./cmd/parser
```

---

## Usage

### Parse a world

```bash
minecraft-bedrock-parser parse <world-path> [flags]
```

Examples:

```bash
# Parse the entire world into world.db
minecraft-bedrock-parser parse ./MyWorld

# Specify an output file
minecraft-bedrock-parser parse ./MyWorld -o /var/www/map/world.db

# Surface blocks only — faster, enough for a 2D map
minecraft-bedrock-parser parse ./MyWorld --surface-only -v

# Live server — copy the world before opening LevelDB
minecraft-bedrock-parser parse ./worlds/MyWorld -o world.db --copy-first

# Overworld only
minecraft-bedrock-parser parse ./MyWorld -d 0
```

| Flag | Description |
|---|---|
| `-o, --output` | Output SQLite file (default: `<world-name>.db`) |
| `-d, --dimension` | `0`=Overworld, `1`=Nether, `2`=End, `-1`=all (default: `-1`) |
| `--region` | Restrict to a chunk region: `x1,z1,x2,z2` |
| `--surface-only` | Export surface blocks only |
| `--copy-first` | Copy the world to a temp directory before opening |
| `-v, --verbose` | Print progress |

### Verify a world

Check that the world parses cleanly before writing any output:

```bash
minecraft-bedrock-parser verify ./MyWorld
```

Prints a diagnostic report without writing any files:

```
World:   "MyWorld"
Seed:    123456789
Spawn:   (0, 64, 0)
Mode:    Survival

Chunks found:    1024
  Overworld:     960
  Nether:        64
  End:           0
SubChunk decode errors: 0 / 15360
Block entity decode errors: 0 / 342
Entity decode errors: 0 / 891

Sample surface blocks (chunk 0,0 overworld):
  (0,0) y=63 minecraft:grass
  (1,0) y=63 minecraft:grass
  ...
```

### Live server usage

LevelDB holds an exclusive file lock, so the parser cannot open a world that is currently in use by the server. The `--copy-first` flag solves this: it copies the world to a temporary directory and parses the copy, leaving the original untouched.

```bash
# cron / systemd timer every 5 minutes:
minecraft-bedrock-parser parse ./worlds/MyWorld -o /var/www/map/world.db --copy-first
```

---

## SQLite schema

Five tables, indexed for coordinate-range queries:

```sql
world_info       -- level name, seed, spawn point, game mode
chunks           -- registry of every parsed chunk
surface_blocks   -- highest non-air block per XZ column
biomes           -- biome IDs (16x16 per chunk)
block_entities   -- chests, signs, furnaces, etc. with full NBT as JSON
entities         -- mobs and other entities with full NBT as JSON
```

### Example queries

Fetch blocks visible in a map viewport (the core dynamic-map query):

```sql
SELECT x, z, y, block_name, block_states
FROM surface_blocks
WHERE dimension = 0
  AND chunk_x BETWEEN -10 AND 10
  AND chunk_z BETWEEN -10 AND 10;
```

Find all chests in the Overworld:

```sql
SELECT x, y, z, data FROM block_entities
WHERE dimension = 0 AND type = 'Chest';
```

World metadata:

```sql
SELECT key, value FROM world_info;
```

---

## World directory locations

| Platform | Path |
|---|---|
| Windows (Microsoft Store) | `%LocalAppData%\Packages\Microsoft.MinecraftUWP_*\LocalState\games\com.mojang\minecraftWorlds\` |
| Windows (Preview) | `%LocalAppData%\Packages\Microsoft.MinecraftWindowsBeta_*\LocalState\games\com.mojang\minecraftWorlds\` |
| Android | `/sdcard/games/com.mojang/minecraftWorlds/` |
| iOS | Via iTunes or iMazing backup |
| Bedrock Dedicated Server | `worlds/<world-name>/` next to `bedrock_server` |

---

## Project structure

```
├── cmd/parser/          # CLI (cobra)
├── pkg/
│   ├── world/           # Bedrock format decoders
│   │   ├── chunk.go     # LevelDB key parsing
│   │   ├── subchunk.go  # SubChunk v8/v9 (bit-packed indices + NBT palette)
│   │   ├── biome.go     # 2D biome decoder
│   │   ├── entity.go    # Entity and block-entity decoders
│   │   ├── metadata.go  # level.dat parser
│   │   └── reader.go    # WorldReader — chunk iteration
│   └── export/
│       └── sqlite.go    # SQLite exporter
├── testdata/
│   ├── generate_fixtures.go  # Generates a minimal test world
│   └── fixtures/             # Binary fixtures for integration tests
└── scripts/
    └── test.sh          # Local test runner
```

---

## Testing

```bash
# Generate test fixtures (one-time)
go run testdata/generate_fixtures.go

# Run all tests
go test ./... -v

# Or use the helper script (fixtures + tests + vet)
./scripts/test.sh
```

GitHub Actions runs `go build`, `go test -race`, and `go vet` on every push.

---

## Known limitations

- **SubChunk v8/v9 only** — legacy formats (pre-1.2) are not supported.
- **3D biomes (1.18+)** — the new `BiomeState` tag format is not yet fully decoded; a 2D fallback is used when available.
- **LevelDB lock** — the parser cannot open a world held open by a running server or game without `--copy-first`.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/df-mc/goleveldb` | LevelDB with Zlib/LZ4 support required by Bedrock |
| `modernc.org/sqlite` | Pure-Go SQLite (no CGo) |
| `github.com/spf13/cobra` | CLI |
