# Siigo - Arquitectura del Middleware

## Objetivo

Servicio que detecta cambios en los archivos ISAM de Siigo y los envia a Finearom (plataforma web Laravel de ordenes de compra B2B).

## Direccion del flujo

```
Siigo (escritorio) -------> Middleware (Go) -------> Finearom (Laravel)
     archivos ISAM          polling + lectura          API HTTP
                             EXTFH / binario            POST/PUT
```

**Unidireccional**: Solo lectura de Siigo -> Web. No escribimos en los archivos ISAM (riesgo de corrupcion).

## Proyectos Implementados

### siigo-sync (CLI middleware)
- **Ubicacion**: `C:\Users\lordmacu\siigo\siigo-sync\`
- **Tecnologia**: Go
- **Ejecucion**: CLI / Windows Service
- **Uso**: Servicio de fondo que hace polling periodico

### siigo-app (Desktop GUI)
- **Ubicacion**: `C:\Users\lordmacu\siigo\siigo-app\`
- **Tecnologia**: Go + Wails (frontend web embebido)
- **Ejecucion**: Aplicacion de escritorio
- **Uso**: Interfaz visual para explorar datos y controlar sincronizacion

## Arquitectura Interna

```
+-------------------------------------------------------+
|              siigo-sync / siigo-app                     |
|                                                         |
|  +---------------+  +--------------+  +-------------+  |
|  |  ISAM Reader   |  |  Change      |  |  HTTP       |  |
|  |  (extfh.go +   |->|  Detector    |->|  Sender     |  |
|  |   reader.go)   |  |  (detector)  |  |  (client)   |  |
|  +---------------+  +--------------+  +-------------+  |
|         |                    |                          |
|    C:\DEMOS01\*        Estado local                     |
|    via EXTFH/binary    (JSON/SQLite)                    |
+-------------------------------------------------------+
```

### Componentes

1. **ISAM Reader** (`isam/extfh.go` + `isam/reader.go`)
   - **EXTFH wrapper**: Llama a `cblrtsm.dll` via syscall con estructura FCD3 (280 bytes)
   - **Binary fallback**: Lee archivos directamente parseando header y marcadores de registro
   - **API unificada**: `ReadIsamFile(path)` intenta EXTFH primero, fallback si no hay DLL
   - Ver `08-EXTFH-WRAPPER.md` para detalles tecnicos

2. **Parsers** (`parsers/`)
   - `terceros.go` - Parse Z17 (clientes/proveedores): NIT, nombre, direccion, telefono
   - `productos.go` - Parse Z06 (productos): codigo, nombre, precio
   - `movimientos.go` - Parse Z49 (transacciones): tipo comprobante, monto, descripcion

3. **Change Detector** (`sync/detector.go`)
   - Polling por timestamp + hash de contenido
   - Compara registros con el ultimo estado conocido

4. **State Store** (`sync/state.go`)
   - Almacena ultimo estado conocido
   - siigo-app usa SQLite (`storage/db.go`), siigo-sync usa JSON

5. **HTTP Client** (`api/client.go`)
   - Envia datos nuevos/modificados a la API de Finearom
   - Autenticacion via Laravel Sanctum (Bearer token)

## Deteccion de Cambios

### Estrategia: Polling por timestamp + hash de contenido

```
Cada N minutos:
  1. Verificar timestamp de ultima modificacion de cada archivo ISAM
  2. Si el timestamp cambio -> re-leer el archivo via ReadIsamFile()
  3. Comparar registros con el ultimo estado conocido (por hash)
  4. Registros nuevos o modificados -> enviar a la web
  5. Actualizar estado local
```

### Ventajas
- No interfiere con Siigo (solo lectura)
- EXTFH maneja archivos bloqueados automaticamente (lock retry)
- Fallback binario funciona sin runtime de Micro Focus

## Datos Sincronizados

| Dato | Archivo Siigo | Endpoint Finearom | Parser |
|------|--------------|-------------------|--------|
| Clientes | Z17 | `POST /api/clients` | `parsers/terceros.go` |
| Productos | Z06 | `POST /api/products` | `parsers/productos.go` |
| Movimientos | Z49 | `POST /api/recaudo/import` | `parsers/movimientos.go` |
| Cartera | Z09YYYY | `POST /api/cartera/import` | (pendiente) |

## Configuracion

```json
{
  "Siigo": {
    "DataPath": "C:\\DEMOS01\\",
    "PollingIntervalSeconds": 60,
    "FilesToWatch": ["Z17", "Z06", "Z49"]
  },
  "WebApi": {
    "BaseUrl": "https://finearom.example.com/api",
    "ApiKey": "xxx",
    "Timeout": 30
  }
}
```

## Consideraciones

### Lectura de Archivos ISAM
- **EXTFH** (preferido): Usa el runtime de Micro Focus para lectura correcta. Requiere `cblrtsm.dll` en el PATH.
- **Binary fallback**: Funciona sin runtime. Valida header (magic 0x33FE/0x30xx), escanea marcadores de registro con validacion de status nibble.
- Los archivos ISAM son pequenos (maximo 7.5MB) — la lectura es rapida.
- EXTFH reintenta automaticamente en caso de file lock (hasta 3 veces, 200ms entre intentos).

### Seguridad
- El servicio corre en la misma maquina que Siigo (acceso local a archivos)
- La comunicacion con la web debe ser HTTPS
- Token Sanctum para autenticacion con Finearom

### Robustez
- Lock retry automatico para archivos bloqueados
- Reintentos con backoff exponencial para envios HTTP fallidos
- Cola de mensajes local si la web no esta disponible
- Logging detallado para diagnostico

### Instalacion
- siigo-sync: Instalar como Windows Service o tarea programada
- siigo-app: Ejecutar como aplicacion de escritorio
- Debe correr en la misma maquina que Siigo

## Estado del Desarrollo

### Completado
- Lector ISAM binario con validacion de header
- Wrapper EXTFH con FCD3, KDB, lock retry, environment auto-setup
- API unificada `ReadIsamFile()` (EXTFH + fallback)
- Parsers para terceros (Z17), productos (Z06), movimientos (Z49)
- siigo-app migroda a usar EXTFH wrapper
- siigo-sync migrodo a usar EXTFH wrapper

### Pendiente
- Parser para cartera (Z09YYYY)
- Detector de cambios (polling loop)
- Conexion HTTP a Finearom en produccion
- Instalacion como Windows Service
