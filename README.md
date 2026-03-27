# DBF-SYNC

> Synchronize dBase III / FoxPro `.dbf` files to MySQL — fast, reliable, and configurable.

**DBF-SYNC** is a terminal UI tool written in Go that reads legacy `.dbf` database files and syncs them into MySQL with three strategies: **upsert**, **append-only**, and **cobrador** (collector update). Built for production use with large datasets (700K+ records).

---

## Features

- **Interactive TUI** — keyboard-driven interface built with Bubbletea
- **Three sync modes** per table:
  - `upsert` — insert new + update existing records
  - `append` — insert only records not already in MySQL
  - `cobrador` — update collector records for a specific month/year
- **Smart filtering** — update window limits updates to current month and specific series
- **Batch engine** — dynamic batch sizing prevents MySQL's 65535 placeholder limit
- **Post-rules** — automatically set field values after insert/update (e.g. `ESTADO=1`)
- **File browser** — navigate directories to select `.dbf` files
- **Cancellable** — press `ESC` during sync to abort
- **Memory efficient** — frees OS memory after large syncs via `debug.FreeOSMemory()`
- **Cross-platform** — Linux and Windows binaries available

---

## Installation

### Download binary

Grab the latest release from [GitHub Releases](https://github.com/Victor0451/dbf-sync/releases):

```bash
# Linux
curl -L https://github.com/Victor0451/dbf-sync/releases/latest/download/dbf-sync-linux-amd64 -o dbf-sync
chmod +x dbf-sync
sudo mv dbf-sync /usr/local/bin/
```

```powershell
# Windows — download dbf-sync-windows-amd64.exe and add to PATH
```

### Build from source

```bash
git clone https://github.com/Victor0451/dbf-sync.git
cd dbf-sync
make install
```

Requires Go 1.22+.

---

## Usage

```bash
dbf-sync interactive
```

That's it. The TUI guides you through:

1. Select database
2. Select table
3. Select `.dbf` file (with directory browser)
4. Select action
5. Confirm and sync

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate |
| `Enter` | Confirm / open |
| `ESC` | Go back / cancel |
| `Backspace` | Up one directory (file browser) |
| `t` | Type path manually (file browser) |
| `q` | Quit |

---

## Configuration

Copy the example config and edit with your database credentials:

```bash
cp config/config.example.yaml config/config.yaml
```

```yaml
databases:
  my_db:
    host: 192.168.1.100
    port: 3306
    user: myuser
    password: "mypassword"
    database: my_database

settings:
  dbf_directories:
    my_db: /path/to/dbf/files/

tables:
  my_table:
    mode: upsert               # upsert | append
    match_keys: [ID]           # composite key for matching records

  payments:
    mode: upsert
    match_keys: [SERIE, NRO_RECIBO, DIA_EMI]
    update_window: current_month   # only update records from current month
    update_date_field: DIA_EMI
    update_series: [2, 22]         # only update records with SERIE in this list
    cobrador:
      serie: [2, 22]

  new_records_only:
    mode: append
    match_keys: [CONTRATO, MES, ANO]

  with_post_rules:
    mode: append
    match_keys: [CONTRATO]
    post_insert:
      - set: {ESTADO: 1}
        description: "Set ESTADO=1 for new records"
    post_update:
      - set: {ESTADO: 1}
        when: "BAJA IS NULL"
      - set: {ESTADO: 0}
        when: "BAJA IS NOT NULL"
```

> **Important:** `config/config.yaml` is gitignored — never commit credentials.

---

## Sync Modes

### `upsert`
Loads all existing keys from MySQL into a hashmap (1 query), classifies every DBF record as INSERT or UPDATE, then executes bulk operations via multi-row INSERT and UPDATE JOIN through a temp table.

### `append`
Same as upsert but `updateFilter` always returns false — existing records are never touched. Only genuinely new records are inserted.

### `cobrador`
Filters DBF records by SERIE and target month/year, then updates only those records in MySQL. Used for monthly collector settlement workflows.

---

## Performance

Tested on a 700K+ record table:

| Operation | Records | Time |
|-----------|---------|------|
| Load existing keys | 616K | ~2s |
| Classify 700K records | 700K | ~7s |
| Bulk insert (new) | varies | ~fast |
| Update JOIN (current month) | ~4K | ~2s |

Batch size is calculated dynamically: `min(65000 / columns, 5000)` to stay under MySQL's 65535 placeholder limit.

---

## Architecture

```
cmd/          — Cobra CLI commands
config/       — YAML config loader
dbf/          — dBase III binary reader (ISO-8859-1 encoding)
mysql/        — Sync engine: upsert, append, cobrador, post-rules
sync/         — Orchestration layer and filter logic
ui/           — Bubbletea TUI: views, model, file browser, styles
```

---

## Requirements

- Go 1.22+
- MySQL 5.7+ / MariaDB 10.3+
- Linux or Windows

---

## License

MIT — see [LICENSE](LICENSE)

---

*Powered by VML PROGRAMMING*
