# Minecraft Bedrock World Parser

Parser światów Minecraft Bedrock Edition jako krok pośredni w pipeline:

```
świat Bedrock  →  minecraft-bedrock-parser  →  SQLite  →  dynamiczna mapa
```

Odczytuje pliki świata z natywnego formatu LevelDB i zapisuje dane do bazy SQLite zoptymalizowanej pod ładowanie widocznych chunków przez dynamiczną mapę (viewport-based loading).

---

## Wymagania

- Go 1.22+
- Świat Minecraft Bedrock Edition (PC/Android/iOS backup)

## Instalacja

```bash
git clone https://github.com/MAK-9/Minecraft-Bedrock-World-Parser
cd Minecraft-Bedrock-World-Parser
go build -o minecraft-bedrock-parser ./cmd/parser
```

---

## Użycie

### Parsowanie świata

```bash
minecraft-bedrock-parser parse <ścieżka-do-świata> [flagi]
```

Przykłady:

```bash
# Parsowanie całego świata do pliku world.db
minecraft-bedrock-parser parse ./MyWorld

# Podaj własną nazwę pliku wyjściowego
minecraft-bedrock-parser parse ./MyWorld -o /var/www/map/world.db

# Tylko powierzchniowe bloki (szybszy tryb, wystarczy dla map 2D)
minecraft-bedrock-parser parse ./MyWorld --surface-only -v

# Live serwer — parser kopiuje świat zanim otworzy LevelDB
minecraft-bedrock-parser parse ./worlds/MyWorld -o world.db --copy-first

# Tylko Overworld
minecraft-bedrock-parser parse ./MyWorld -d 0
```

| Flaga | Opis |
|---|---|
| `-o, --output` | Plik wyjściowy SQLite (domyślnie: `<nazwa-świata>.db`) |
| `-d, --dimension` | `0`=Overworld, `1`=Nether, `2`=End, `-1`=wszystkie (domyślnie: `-1`) |
| `--region` | Ogranicz do regionu chunków: `x1,z1,x2,z2` |
| `--surface-only` | Tylko bloki powierzchniowe (szybszy tryb dla map 2D) |
| `--copy-first` | Kopiuje świat do katalogu tymczasowego przed otwarciem |
| `-v, --verbose` | Szczegółowe logi postępu |

### Walidacja świata

Przed parsowaniem możesz sprawdzić czy świat jest poprawnie odczytywany:

```bash
minecraft-bedrock-parser verify ./MyWorld
```

Wypisuje raport bez zapisywania żadnych plików:

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

### Live serwer — aktualizacja co N minut

LevelDB używa wyłącznej blokady pliku, więc parser **nie może** bezpośrednio otworzyć świata, który jest aktualnie używany przez serwer. Flaga `--copy-first` rozwiązuje ten problem — parser tworzy kopię świata i otwiera ją, nie dotykając oryginału.

```bash
# cron lub systemd timer co 5 minut:
minecraft-bedrock-parser parse ./worlds/MyWorld -o /var/www/map/world.db --copy-first
```

---

## Schemat SQLite

Baza zawiera pięć tabel zoptymalizowanych do zapytań po koordynatach:

```sql
world_info       -- metadane: nazwa świata, seed, spawn, tryb gry
chunks           -- rejestr sparsowanych chunków
surface_blocks   -- najwyższy niepowietrzny blok na każdej kolumnie XZ
biomes           -- dane biomów (16×16 per chunk)
block_entities   -- skrzynie, znaki, piece itp. z pełnymi danymi NBT
entities         -- moby i inne encje z pełnymi danymi NBT
```

### Przykładowe zapytania

Pobierz bloki widoczne w viewporcie mapy (klucz do dynamicznej mapy):

```sql
SELECT x, z, y, block_name, block_states
FROM surface_blocks
WHERE dimension = 0
  AND chunk_x BETWEEN -10 AND 10
  AND chunk_z BETWEEN -10 AND 10;
```

Znajdź wszystkie skrzynie w Overworld:

```sql
SELECT x, y, z, data FROM block_entities
WHERE dimension = 0 AND type = 'Chest';
```

Metadane świata:

```sql
SELECT key, value FROM world_info;
```

---

## Lokacja plików świata

| Platforma | Ścieżka |
|---|---|
| Windows (Microsoft Store) | `%LocalAppData%\Packages\Microsoft.MinecraftUWP_*\LocalState\games\com.mojang\minecraftWorlds\` |
| Windows (Preview) | `%LocalAppData%\Packages\Microsoft.MinecraftWindowsBeta_*\LocalState\games\com.mojang\minecraftWorlds\` |
| Android | `/sdcard/games/com.mojang/minecraftWorlds/` |
| iOS | Backup przez iTunes / iMazing |
| Serwer Bedrock (BDS) | `worlds/<nazwa-świata>/` obok `bedrock_server` |

---

## Struktura projektu

```
├── cmd/parser/          # CLI (cobra)
├── pkg/
│   ├── world/           # Decodery formatu Bedrock
│   │   ├── chunk.go     # Parsowanie kluczy LevelDB
│   │   ├── subchunk.go  # Dekoder SubChunk v8/v9 (bloki + palette NBT)
│   │   ├── biome.go     # Dekoder biom 2D
│   │   ├── entity.go    # Dekoder encji i block entities
│   │   ├── metadata.go  # Parser level.dat
│   │   └── reader.go    # WorldReader — iteracja chunków
│   └── export/
│       └── sqlite.go    # Eksport do SQLite
├── testdata/
│   ├── generate_fixtures.go  # Generator minimalnego świata testowego
│   └── fixtures/             # Binarne fixtures do testów integracyjnych
└── scripts/
    └── test.sh          # Lokalny runner testów
```

---

## Testy

```bash
# Wygeneruj fixtures testowe (jednorazowo)
go run testdata/generate_fixtures.go

# Uruchom wszystkie testy
go test ./... -v

# Lub użyj skryptu (fixtures + testy + vet)
./scripts/test.sh
```

CI (GitHub Actions) uruchamia `go build`, `go test -race` i `go vet` przy każdym pushu.

---

## Znane ograniczenia

- **SubChunk v8/v9 only** — starsze formaty (pre-1.2) nie są obsługiwane.
- **3D biomy (1.18+)** — nowy format biom (`BiomeState` tag) nie jest jeszcze w pełni dekodowany; zapisywana jest 2D reprezentacja gdy dostępna.
- **LevelDB lock** — parser nie może otworzyć świata aktywnie używanego przez serwer/grę bez flagi `--copy-first`.

---

## Zależności

| Pakiet | Rola |
|---|---|
| `github.com/df-mc/goleveldb` | LevelDB z obsługą kompresji Zlib/LZ4 (wymaganej przez Bedrock) |
| `modernc.org/sqlite` | SQLite pure-Go (bez CGo) |
| `github.com/spf13/cobra` | CLI |
