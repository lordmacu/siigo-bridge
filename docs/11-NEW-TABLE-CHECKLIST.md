# Checklist: Agregar una Nueva Tabla al Sistema

Guia paso a paso para integrar una nueva tabla ISAM con toda la funcionalidad:
dashboard, API v1, OData, sync bidireccional, Postman, frontend, y Swagger.

---

## Pre-requisitos

- [ ] **Hex dump del archivo ISAM** para identificar offsets
  ```bash
  cd siigo-sync && go run ./cmd/hexdump/ 'C:\Archivos Siigo\ZXXX'
  ```
- [ ] **Identificar campos**: nombres, offsets, tipos (ASCII, BCD, bool)
- [ ] **Identificar clave primaria**: que campo(s) hacen unico a cada registro
- [ ] **Identificar FKs**: relaciones con clients (nit), products (code), plan_cuentas (codigo_cuenta), etc.

---

## Paso 1: Modelo ISAM (siigo-common/isam/models.go)

- [ ] Crear variable de modelo con `DefineModel()`

```go
var MiTabla = DefineModel("mi_tabla", "ZXXX", true, "", 1024, func(m *Model) {
    m.String("campo1", offset, length)
    m.String("campo2", offset, length)
    // m.BCD("valor", offset, length, decimals)  // para campos packed decimal
})
```

**Parametros de DefineModel:**
| Param | Descripcion |
|-------|-------------|
| `"mi_tabla"` | Nombre interno (snake_case) |
| `"ZXXX"` | Prefijo archivo ISAM |
| `true/false` | hasYear — si el archivo usa sufijo YYYY (ej: Z092012) |
| `""` | Sufijo extra (ej: "A" para Z08A, "CP11" para Z279CP11) |
| `1024` | Record length en bytes |

- [ ] Registrar en `ConnectAll()` (misma funcion, al final)

---

## Paso 2: Tabla SQLite (siigo-common/storage/db.go)

- [ ] Agregar a `validTables` map (~linea 20)
```go
"mi_tabla": true,
```

- [ ] Crear sentencia `CREATE TABLE` en el slice de migraciones (~linea 1060+)
```go
`CREATE TABLE IF NOT EXISTS mi_tabla (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    record_key TEXT UNIQUE,
    campo1 TEXT DEFAULT '',
    campo2 TEXT DEFAULT '',
    valor REAL DEFAULT 0,
    hash TEXT DEFAULT '',
    sync_status TEXT DEFAULT 'pending',
    sync_action TEXT DEFAULT 'new',
    sync_error TEXT DEFAULT '',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME,
    retry_count INTEGER DEFAULT 0
)`,
`CREATE INDEX IF NOT EXISTS idx_mi_tabla_status ON mi_tabla(sync_status)`,
```

- [ ] (Opcional) Agregar indexes de FK si tiene relaciones
```go
`CREATE INDEX IF NOT EXISTS idx_mi_tabla_nit ON mi_tabla(nit_tercero)`,
```

- [ ] (Opcional) Agregar VIEW si enriquece datos con JOINs
```go
`CREATE VIEW IF NOT EXISTS v_mi_tabla_detalle AS
    SELECT t.*, cl.nombre AS nombre_tercero
    FROM mi_tabla t
    LEFT JOIN clients cl ON CAST(CAST(cl.nit AS INTEGER) AS TEXT) = CAST(CAST(t.nit_tercero AS INTEGER) AS TEXT)`,
```

---

## Paso 3: Config (siigo-common/config/config.go)

- [ ] Agregar a `allSyncTables` (~linea 206)
```go
var allSyncTables = []string{
    // ... tablas existentes ...
    "mi_tabla",
}
```

Esto automaticamente habilita:
- Toggle detect/send per-table
- Defaults en `DefaultSendEnabled()` y `DefaultDetectEnabled()`
- Field mappings base

---

## Paso 4: Sync Registry (siigo-web/main.go — initSyncTables)

- [ ] Agregar entrada en `syncRegistry` (~linea 920+)

```go
"mi_tabla": {
    Table: "mi_tabla",
    Model: isam.MiTabla,
    Label: "Mi Tabla",
    KeyCol: "record_key",
    KeyFunc: func(r *isam.Row) string {
        // Construir clave unica
        c := strings.TrimSpace(r.Get("campo1"))
        if c == "" { return "" }
        return c
    },
    ColMap: map[string]string{
        "campo1":  "campo1",    // SQLite col → ISAM field
        "campo2":  "campo2",
        // Prefijo ~ para floats/BCD:
        // "~valor": "valor",
    },
    // BoolMap: map[string]string{"activo": "activo"},  // S/N → 1/0
    // ComputedCols: map[string]func(map[string]string) string{
    //     "saldo_final": func(row map[string]string) string { ... },
    // },
},
```

---

## Paso 5: Detect Order (siigo-web/main.go)

- [ ] Agregar a `detectOrder` (~linea 1500)
```go
detectOrder := []string{
    // ... tablas existentes ...
    "mi_tabla",
}
```

---

## Paso 6: Setup Wizard (siigo-web/main.go)

- [ ] Agregar a `setupTablesList` (~linea 1643)
```go
{"mi_tabla", "Mi Tabla (ZXXX)"},
```

---

## Paso 7: OData (siigo-web/main.go)

- [ ] Agregar a `odataTables` map (~linea 3829)
```go
"mi_tabla": {EntityType: "MiTabla", KeyProp: "record_key"},
```

- [ ] Agregar a `odataTableOrder` (~linea 3863)
```go
"mi_tabla",
```

- [ ] (Si tiene FK) Agregar a `odataRelations` (~linea 3884)
```go
{"mi_tabla", "nit_tercero", "clients", "nit", "Client"},
{"mi_tabla", "cuenta_contable", "plan_cuentas", "codigo_cuenta", "CuentaContable"},
```

---

## Paso 8: Postman Collection (siigo-web/main.go)

- [ ] Agregar a `tableDescriptions` (~linea 4916)
```go
"mi_tabla": "Mi Tabla (ZXXX)",
```

---

## Paso 9: Ruta API Dashboard (siigo-web/main.go)

- [ ] Registrar ruta `/api/mi-tabla` (~linea 786+)
```go
mux.HandleFunc("/api/mi-tabla", s.permMiddleware(s.handleGenericTable("mi_tabla")))
```

> **Convencion**: URL usa kebab-case (`mi-tabla`), tabla SQLite usa snake_case (`mi_tabla`)

---

## Paso 10: Frontend — API (siigo-web/frontend/src/api.ts)

- [ ] Agregar funcion getter
```typescript
getMiTabla: (page: number, search: string) => get(`/mi-tabla?page=${page}&search=${encodeURIComponent(search)}`),
```

---

## Paso 11: Frontend — Table Config (siigo-web/frontend/src/config/tables.ts)

- [ ] Agregar configuracion de columnas
```typescript
mi_tabla: {
  apiPath: 'mi-tabla',
  columns: [
    { key: 'campo1', label: 'Campo 1', type: 'key' },
    { key: 'campo2', label: 'Campo 2', type: 'name' },
    { key: 'valor', label: 'Valor', type: 'value' },
    // tipos: key, name, date, code, type, desc, value
  ],
},
```

---

## Paso 12: Frontend — Sidebar (siigo-web/frontend/src/components/Sidebar.tsx)

- [ ] Agregar item de navegacion en `navItems` (~linea 5+)
```typescript
{ path: '/mi-tabla', label: 'Mi Tabla', badge: 'ZXXX', module: 'mi_tabla' },
```

> Si `badge` esta vacio, el sidebar NO oculta la tabla aunque tenga 0 registros.
> Las tablas con badge se ocultan automaticamente si tienen 0 registros.

---

## Paso 13: Frontend — Ruta (siigo-web/frontend/src/App.tsx)

- [ ] Agregar Route
```tsx
<Route path="/mi-tabla" element={guard('mi_tabla', <DataPage table="mi_tabla" title="Mi Tabla (ZXXX)" file="ZXXX" />)} />
```

---

## Paso 14: Swagger (siigo-web/swagger.json)

- [ ] Agregar tag
```json
{"name": "MiTabla", "description": "Mi Tabla - descripcion (ZXXX)"}
```

- [ ] Agregar endpoints list + detail
```json
"/api/v1/mi_tabla": {
  "get": {
    "tags": ["MiTabla"],
    "summary": "Listar mi tabla",
    "description": "Campos: campo1, campo2, valor. FK: nit_tercero → clients.nit.",
    "security": [{"bearerAuth": []}],
    "parameters": [
      {"$ref": "#/components/parameters/page"},
      {"$ref": "#/components/parameters/limit"},
      {"$ref": "#/components/parameters/search"},
      {"$ref": "#/components/parameters/sync_status"},
      {"$ref": "#/components/parameters/since"}
    ],
    "responses": {
      "200": {"description": "Lista paginada", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/PaginatedResponse"}}}},
      "401": {"$ref": "#/components/responses/Unauthorized"}
    }
  }
},
"/api/v1/mi_tabla/{key}": {
  "get": {
    "tags": ["MiTabla"],
    "summary": "Detalle",
    "security": [{"bearerAuth": []}],
    "parameters": [{"name": "key", "in": "path", "required": true, "schema": {"type": "string"}, "description": "record_key"}],
    "responses": {
      "200": {"description": "Detalle", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Record"}}}},
      "404": {"$ref": "#/components/responses/NotFound"}
    }
  }
}
```

- [ ] Actualizar enum de OData table si es necesario
- [ ] Actualizar conteo de tablas en descripciones (ej: "27 tablas" → "28 tablas")

---

## Paso 15: Documentacion

- [ ] Actualizar [docs/10-TABLE-RELATIONSHIPS.md](10-TABLE-RELATIONSHIPS.md) con las nuevas FK
- [ ] Actualizar CLAUDE.md si es una tabla importante
- [ ] Actualizar MEMORY.md con el nuevo parser

---

## Paso 16: Validacion

- [ ] **Build**: `cd siigo-web && go build -o /tmp/test.exe ./...`
- [ ] **Restart**: `./start.sh restart`
- [ ] **Verificar datos**: Abrir dashboard, ver que la tabla aparece en sidebar
- [ ] **Verificar API v1**: `GET /api/v1/mi_tabla?page=1`
- [ ] **Verificar OData**: `GET /odata/mi_tabla?$top=5`
- [ ] **Verificar Swagger**: `GET /api/v1/docs` — ver que aparece la nueva tabla
- [ ] **Verificar Postman**: `GET /api/v1/postman` — ver que incluye la tabla

---

## Resumen de Archivos a Modificar

| # | Archivo | Que agregar |
|---|---------|-------------|
| 1 | `siigo-common/isam/models.go` | DefineModel + campo definitions |
| 2 | `siigo-common/storage/db.go` | validTables + CREATE TABLE + indexes + views |
| 3 | `siigo-common/config/config.go` | allSyncTables |
| 4 | `siigo-web/main.go` | syncRegistry, detectOrder, setupTablesList, odataTables, odataTableOrder, odataRelations, tableDescriptions, ruta API |
| 5 | `siigo-web/frontend/src/api.ts` | getter function |
| 6 | `siigo-web/frontend/src/config/tables.ts` | column config |
| 7 | `siigo-web/frontend/src/components/Sidebar.tsx` | nav item |
| 8 | `siigo-web/frontend/src/App.tsx` | Route |
| 9 | `siigo-web/swagger.json` | tag + endpoints + OData enum |
| 10 | `docs/10-TABLE-RELATIONSHIPS.md` | FK si aplica |

---

## Que NO necesitas modificar

- **handleStats** — auto-detecta tablas desde SQLite
- **GetPendingGeneric** — funciona con cualquier tabla en validTables
- **UpsertGeneric** — funciona con cualquier tabla en validTables
- **sendPending** — tiene default case para tablas sin handler especifico
- **OData $metadata** — genera XML dinamicamente desde odataTables + columnas SQLite
- **Sidebar hide logic** — automatico basado en `/api/stats`
