# Siigo Pyme - Ingenieria Inversa y Middleware

## Que es Siigo?

Siigo Pyme es un software de contabilidad y ERP colombiano usado por miles de empresas. Maneja contabilidad, facturacion, inventario, cartera, nomina, activos fijos, y mas.

## Que estamos haciendo?

Ingenieria inversa para construir un **middleware** que lea datos de Siigo y los envie a **Finearom** (plataforma web Laravel de ordenes de compra B2B). Siigo NO tiene una API publica, asi que tuvimos que descifrar como almacena los datos internamente.

## Proyectos Construidos

### 1. siigo-sync (CLI middleware)
- **Ubicacion**: `C:\Users\lordmacu\siigo\siigo-sync\`
- **Tecnologia**: Go (CLI)
- **Funcion**: Servicio de sincronizacion Siigo -> Finearom via polling
- **Componentes**:
  - `isam/` - Lector ISAM con EXTFH wrapper + fallback binario
  - `parsers/` - Parsers de terceros (Z17), productos (Z06), movimientos (Z49)
  - `sync/` - Detector de cambios y estado local
  - `api/` - Cliente HTTP para API Finearom
  - `config/` - Configuracion del servicio

### 2. siigo-app (Wails desktop app)
- **Ubicacion**: `C:\Users\lordmacu\siigo\siigo-app\`
- **Tecnologia**: Go + Wails (desktop GUI)
- **Funcion**: Aplicacion de escritorio para explorar y sincronizar datos
- **Componentes**: Misma estructura que siigo-sync + `storage/db.go` (SQLite) + `app.go` (Wails bindings)

### 3. SiigoExplorer (exploracion inicial)
- **Ubicacion**: `C:\Users\lordmacu\siigo\SiigoExplorer\`
- **Tecnologia**: .NET Framework 4.8, x86
- **Funcion**: Exploracion inicial de DLLs .NET de Siigo (ya no se usa activamente)

## Que descubrimos?

### La sorpresa: Siigo NO usa una base de datos SQL

Siigo esta escrito en **COBOL** (Micro Focus COBOL Server 9.0). Los datos se almacenan en archivos **ISAM** (Indexed Sequential Access Method) — un formato binario propietario donde cada "tabla" son 2 archivos: uno de datos y uno de indices.

### Stack Tecnologico de Siigo

```
+---------------------------------------------------+
|  Interfaz de Usuario (Windows Forms / COBOL UI)    |
+---------------------------------------------------+
|  DLLs .NET (wrappers limitados)                    |
|  - SIIGOCN.dll (Business Objects)                  |
|  - SIIGOCV.dll (DTOs, Session)                     |
+---------------------------------------------------+
|  Programas COBOL nativos (.gnt)                    |
|  - ~3205 programas compilados                      |
|  - BOINV, BOMOV, BOMAE, BOCLINT, etc.             |
+---------------------------------------------------+
|  Micro Focus COBOL Runtime 9.0                     |
|  - Extended File Handler (EXTFH)                   |
|  - IDXFORMAT=8                                     |
+---------------------------------------------------+
|  Archivos ISAM (datos binarios)                    |
|  - Z17, Z06, Z49, C03, etc.                       |
|  - Cada archivo = datos + .idx (indice)            |
+---------------------------------------------------+
```

### Nuestro Middleware (Go)

```
+---------------------------------------------------+
|  siigo-app (Wails GUI) / siigo-sync (CLI)          |
+---------------------------------------------------+
|  EXTFH Wrapper (Go -> cblrtsm.dll via syscall)     |
|  - FCD3 struct (280 bytes, 64-bit)                 |
|  - KDB (Key Definition Block)                      |
|  - Lock retry, environment auto-setup              |
+---------------------------------------------------+
|  Fallback: Binary Reader (Go)                      |
|  - Header validation (magic 0x33FE/0x30xx)         |
|  - Record marker scanning                          |
|  - Status nibble validation                        |
+---------------------------------------------------+
|  Parsers (terceros, productos, movimientos)        |
|  -> API HTTP -> Finearom (Laravel)                 |
+---------------------------------------------------+
```

### Tres caminos que investigamos

1. **DLLs .NET (SIIGOCN.dll, SIIGOCV.dll)** -> Tienen Business Objects y DTOs pero los DTOs NO exponen datos como propiedades .NET. Los datos quedan en memoria COBOL interna. **DESCARTADO para lectura de datos**.

2. **ExecuteCall (invocar programas COBOL)** -> Requiere `NativeLauncher.dll` que es una DLL nativa (C/C++), no se puede cargar desde .NET externo. **DESCARTADO**.

3. **EXTFH (Callable File Handler)** -> **FUNCIONA**. Llamamos directamente a `cblrtsm.dll` via syscall con la estructura FCD3. Si la DLL no esta disponible, usamos lectura binaria directa como fallback. Ver `08-EXTFH-WRAPPER.md`.

### Rutas en el sistema

| Ruta | Que contiene |
|------|-------------|
| `C:\Siigo\` | Instalacion: programas .gnt, DLLs, configuracion |
| `C:\SIIWI02\` | Datos de la empresa demo (~957 archivos) |
| `C:\Siigo\FILEPATH.TXT` | Contiene la ruta al directorio de datos activo |
| `C:\Siigo\EXTFH.CFG` | Configuracion del File Handler ISAM |
| `C:\Siigo\COBCONFIG.CFG` | Config COBOL: `set no_mfredir=TRUE` |
| `C:\Siigo\SIIGO.CFG` | Version (91), URLs de contrato |
| `C:\ProgramData\Micro Focus\` | Licencias y servicios Micro Focus |

## Documentos en este directorio

| Archivo | Contenido |
|---------|-----------|
| `01-OVERVIEW.md` | Este documento - vision general |
| `02-ISAM-FORMAT.md` | Formato binario de los archivos ISAM y como leerlos |
| `03-DATA-DICTIONARY.md` | Diccionario de datos: cada archivo, que contiene, estructura de campos |
| `04-DOTNET-API.md` | API de las DLLs .NET (limitada pero util para referencia) |
| `05-SETUP-AND-FIXES.md` | Instalacion, licencias, problemas resueltos |
| `06-MIDDLEWARE-STRATEGY.md` | Arquitectura del middleware Go (siigo-sync + siigo-app) |
| `07-SYNC-MAP-FINEAROM.md` | Mapeo de sincronizacion Siigo <-> Finearom |
| `08-EXTFH-WRAPPER.md` | Wrapper EXTFH: FCD3, KDB, opcodes, lock retry |

## Datos de ejemplo que logramos leer

**Terceros (Z17):**
- "PROVEEDORES"
- "SUPERMERCADOS LA GRAN ESTRELLA"
- Seguros contra incendio, contra terremoto
- Mantenimiento software

**Movimientos (Z49):**
- "CRUCE DE CUENTAS S/N ANEXO"
- "ABONO A FACT. CTA 01"
- "ANTICIPO DE CLIENTE. CTA 01"
- "CANCELA FACT. Y SALDO A FAVOR DEL CLIENTE"

**Facturacion electronica (Z90ES):**
- "Facturacion electronica"
- "Elabora tu factura de venta electronica"
- "Elabora notas credito/debito electronicas"
- "Documento soporte electronico"

**Comprobantes (Z70):**
- "FAVOR COMUNICARSE CON EL CLIENTE Y ACUERDO PAGO"
- "LLAMAR NUEVAMTE"
