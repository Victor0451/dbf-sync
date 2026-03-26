# Changelog

Todos los cambios notables de este proyecto se documentan aquí.

Formato basado en [Keep a Changelog](https://keepachangelog.com/).
Versionado semántico: `v{major}.{minor}.{patch}`

---

## v0.1.0 — 2026-03-26

### Agregado
- **Lector de archivos .dbf** — Soporte para dBASE III/FoxPro con conversión latin1 → UTF-8
- **Sync optimizado con hash map** — Carga todas las keys de MySQL en 1 query, clasifica registros como INSERT/UPDA TE. 700K registros en ~15 segundos (60x más rápido que el approach por registro)
- **Modo interactivo** — Menú con selección de base de datos, tabla y acción
  - Insertar registros nuevos
  - Actualizar cobradores por mes/año (segmentación: 47 registros vs 700K)
  - Sync completo
- **Post-procesamiento por tabla**
  - `maestro`: fuerza `ESTADO = 1` después de insertar
  - `adherent`: `ESTADO = 1` si `BAJA IS NULL`, `ESTADO = 0` si `BAJA IS NOT NULL`
- **Resumen con antes/después** — Muestra cantidad de registros antes y después de cada sync
- **Configuración dinámica** — Todas las reglas de tablas en `config.yaml`, sin tocar código
- **CLI directo** — Comandos `sync`, `status`, `interactive` con cobra
- **Soporte multi-base** — wercho y sanvalentin con .dbf de carpetas diferentes
- **Banner** — DBF Sync, Powered by VML PROGRAMMING

### Tablas soportadas
| Tabla | Modo | Match Keys |
|-------|------|------------|
| pagos | upsert | SERIE, NRO_RECIBO, DIA_EMI |
| pago_bco | append | id |
| maestro | append | CONTRATO |
| adherent | upsert | CONTRATO, NRO_DOC |
| cuo_fija | append | CONTRATO |
| bajas | append | CONTRACT |

### Bugs fixeados
- `getColumnsForTable()` no filtraba por `TABLE_SCHEMA` — causaba errores al sincronizar sanvalentin
- Ctrl+C no salía del modo interactivo — survey capturaba la señal

### Dependencias
- `github.com/spf13/cobra` — CLI framework
- `github.com/go-sql-driver/mysql` — MySQL driver
- `golang.org/x/text` — Conversión de encoding
- `gopkg.in/yaml.v3` — Parser de config YAML
- `github.com/AlecAivazis/survey/v2` — Menús interactivos
