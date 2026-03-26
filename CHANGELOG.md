# Changelog

Todos los cambios notables de este proyecto se documentan aquí.

Formato basado en [Keep a Changelog](https://keepachangelog.com/).
Versionado semántico: `v{major}.{minor}.{patch}`

---

## v0.2.0 — 2026-03-26

### Agregado
- **TUI completa con Bubbletea** — Interfaz de terminal profesional con lipgloss
  - Pantalla completa con `tea.WithAltScreen()`
  - Keybindings universales: `q` salir, `Esc` volver, `↑/↓` navegar, `Enter` confirmar
  - Spinner animado durante procesamiento
  - Progress bar con porcentaje y ETA
- **Menú principal** — Opciones: Actualizar bases, Ver estado, Instalar, Configuración, Salir
- **Navegador de archivos integrado** — Seleccionar `.dbf` navegando directorios dentro de la TUI
  - Muestra archivos `.dbf` con tamaño y fecha
  - Navegación con `↑/↓`, `Enter` para abrir carpeta/seleccionar, `Backspace` para volver
  - Fallback a `$HOME` (Linux) o `C:\` (Windows) si el directorio configurado no existe
  - Opción `t` para escribir ruta manual
- **Instalador cross-platform** — Copia el binario a:
  - Linux: `~/.local/bin/dbf-sync`
  - Windows: `%APPDATA%\dbf-sync\dbf-sync.exe`
  - Verifica si el directorio está en PATH
- **Configuración de directorios .dbf** — Nuevo campo `settings.dbf_directories` en `config.yaml`
  - Permite configurar directorio por base de datos
  - Soporta paths de Windows (`C:/BASES/...`)
- **Sin args abre TUI** — `dbf-sync` sin subcomandos abre el modo interactivo

### Cambiado
- **Migrado de survey/v2 a Bubbletea** — Mejor manejo de terminal, Ctrl+C funciona correctamente
- **Modo interactivo** ahora usa `tea.NewProgram` con state machine completa
- **Binario más pequeño** — 8.0MB (vs 8.3MB en v0.1.0)

### Removido
- `github.com/AlecAivazis/survey/v2` — Reemplazado por Bubbletea

### Dependencias
- `charm.land/bubbletea/v2` — Framework TUI
- `charm.land/bubbles/v2` — Componentes (spinner, progress, list, textinput)
- `charm.land/lipgloss/v2` — Estilos y colores

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
