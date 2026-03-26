# Plan: DBF Sync → Bubbletea TUI Refactor

## Objetivo
Reemplazar el modo interactivo actual (survey/v2) por una TUI completa con Bubbletea.

## Problemas actuales
1. No se puede salir con Ctrl+C limpiamente
2. Pantallas básicas sin estilo
3. Sin feedback visual durante procesamiento (progress bar no se actualiza)
4. Encuesta captura terminal en raw mode, rompe el signal handling

## Stack nuevo
```
github.com/charmbracelet/bubbletea   → Framework TUI
github.com/charmbracelet/bubbles     → Componentes (spinner, progress, list, textinput)
github.com/charmbracelet/lipgloss    → Estilos/colores
```

## Arquitectura del modelo

```go
type AppState int
const (
    StateSelectDB AppState = iota
    StateSelectTable
    StateSelectAction
    StateInputPath
    StateInputMonth
    StateConfirm
    StateProcessing
    StateSummary
    StateQuit
)

type AppModel struct {
    state       AppState
    config      *config.Config
    db          string
    table       string
    action      string
    dbfPath     string
    month       int
    year        int
    spinner     spinner.Model
    progress    progress.Model
    list        list.Model
    textInput   textinput.Model
    result      *SyncResult
    err         error
    width       int
    height      int
}
```

## Flujo de navegación

```
StateSelectDB → StateSelectTable → StateSelectAction
                                          ├── Insertar → StateInputPath → StateConfirm → StateProcessing → StateSummary
                                          ├── Cobradores → StateInputMonth → StateInputPath → StateConfirm → StateProcessing → StateSummary
                                          ├── Sync completo → StateInputPath → StateConfirm → StateProcessing → StateSummary
                                          ├── Volver → StateSelectTable
                                          └── Salir → StateQuit

En CUALQUIER estado: Esc = volver atrás, Ctrl+C / q = salir
```

## Key bindings universales
- `↑/↓` o `j/k` → navegar listas
- `Enter` → confirmar selección
- `Esc` → volver al paso anterior
- `q` → salir (con confirmación)
- `Ctrl+C` → salir inmediato

## Estructura de archivos

```
cmd/
  interactive.go      → REEMPLAZAR: solo init + tea.NewProgram
ui/
  model.go            → AppModel + Init/Update/View (maquina de estados)
  views.go            → Funciones de renderizado por estado
  styles.go           → Definiciones lipgloss (colores, bordes, etc.)
  components.go       → Componentes reusables (banner, summary box, etc.)
```

## Vistas

### Banner (siempre visible arriba)
```
╔═══════════════════════════════════════════════════╗
║  ██████╗ ██████╗ ███████╗    ███████╗██╗   ██╗   ║
║  ...                                               ║
║          Sincroniza .dbf → MySQL                  ║
║       Powered by VML PROGRAMMING 🐉⚡              ║
╚═══════════════════════════════════════════════════╝
```

### Selección de DB
```
  Paso 1: Seleccionar Base de Datos

  ► wercho
    sanvalentin

  ↑/↓ navegar · Enter confirmar · q salir
```

### Procesando (spinner + progress)
```
  ⠋ Insertando registros...

  [████████████████████░░░░░░░░░░] 67% (470/700) ETA: 2s
```

### Resumen
```
╔══════════════════════════════════════════╗
║         RESUMEN DE SINCRONIZACIÓN        ║
╠══════════════════════════════════════════╣
║  Base de datos:  wercho                  ║
║  Tabla:          cuo_fija                ║
║  Acción:         Insertar nuevos         ║
╠══════════════════════════════════════════╣
║  📊 ANTES:       0 registros             ║
║  ✅ Insertados:  948                     ║
║  📊 DESPUÉS:     948 registros           ║
║  ❌ Errores:     0                       ║
╠══════════════════════════════════════════╣
║  Duración:       0.8s                    ║
╚══════════════════════════════════════════╝

  Enter para continuar · q salir
```

## Pasos de implementación

### 1. Agregar dependencias
```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/bubbles
go get github.com/charmbracelet/lipgloss
```

### 2. Crear ui/styles.go
- Colores: cyan para headers, green para success, red para errors, yellow para warnings
- Bordes con lipgloss.RoundedBorder()
- Padding y márgenes consistentes

### 3. Crear ui/model.go
- AppModel con todos los estados
- Init() → muestra banner + state SelectDB
- Update() → switch por state actual, maneja key messages
- View() → switch por state, renderiza vista correspondiente

### 4. Crear ui/views.go
- viewBanner() → ASCII art con lipgloss
- viewSelectDB() → lista de bases de datos
- viewSelectTable() → lista de tablas
- viewSelectAction() → lista de acciones
- viewInputPath() → text input para ruta .dbf
- viewInputMonth() → inputs para mes/año
- viewConfirm() → confirmación con resumen
- viewProcessing() → spinner + progress bar
- viewSummary() → resumen con before/after

### 5. Crear ui/components.go
- Banner() → renderiza el banner ASCII
- SummaryBox() → renderiza el cuadro de resumen
- StepIndicator() → muestra "Paso X de Y"

### 6. Refactorizar cmd/interactive.go
- Eliminar TODO el código de survey
- Mantener: init() para registrar comando, runInteractive() minimal
- runInteractive() crea tea.NewProgram con AppModel y corre

### 7. Mantener compatibilidad
- cmd/sync.go y cmd/status.go SIN CAMBIOS
- config/config.yaml SIN CAMBIOS
- mysql/ package SIN CAMBIOS
- ui/progress.go y ui/summary.go pueden ser reemplazados por los de Bubbletea

## Testing
1. `./dbf-sync interactive` → navegar todos los menús
2. Ctrl+C en cada estado → debe salir limpiamente
3. Esc en cada estado → debe volver atrás
4. Insertar cuo_fija en sanvalentin → debe funcionar (bug fix)
5. Actualizar cobradores por mes → debe filtrar y mostrar progreso
6. Verificar resumen con antes/después

## Prioridad
ALTA — Este refactor mejora significativamente la experiencia de uso y resuelve
los problemas de Ctrl+C de raíz (Bubbletea maneja el terminal correctamente).
