# Proceso de Parseo de Tablas ISAM de Siigo

Este documento describe en detalle el proceso que seguimos para descubrir la estructura interna de los archivos ISAM de Siigo Pyme, identificar las columnas de cada tabla y construir los parsers que extraen los datos.

---

## 1. Contexto: Por que es necesario este proceso

Siigo Pyme es un sistema contable escrito en COBOL que almacena todos sus datos en archivos ISAM (Indexed Sequential Access Method) con formato Micro Focus IDXFORMAT=8. **No existe documentacion publica** de la estructura interna de estos archivos. Los archivos no tienen schema, headers descriptivos, ni metadatos que indiquen que campos contienen.

Cada "tabla" es un par de archivos binarios:
- `Z17` (datos) + `Z17.idx` (indices)
- `Z09YYYY` (datos) + `Z09YYYY.idx` (indices)

Los registros son de **longitud fija** con campos COBOL (PIC X, PIC 9, packed decimal) sin delimitadores.

---

## 2. Herramientas construidas para el analisis

### 2.1 EXTFH Wrapper (`siigo-common/isam/extfh.go`)
Wrapper en Go que llama a `cblrtsm.dll` (runtime Micro Focus COBOL) via syscall usando la interfaz FCD3 (File Control Description, 280 bytes). Esto permite leer los archivos ISAM usando el mismo motor que usa Siigo.

**Ventaja sobre lectura binaria directa**: EXTFH entrega registros "limpios" sin los marcadores de 2 bytes que el formato binario incluye entre registros.

### 2.2 Binary Reader fallback (`siigo-common/isam/reader.go`)
Lector binario que parsea directamente la estructura del archivo ISAM cuando la DLL no esta disponible. Busca marcadores de registro (status nibble + record size) y extrae los datos crudos.

### 2.3 Hexdump tool (`siigo-sync/cmd/hexdump/main.go`)
Herramienta que lee registros ISAM y muestra:
- Los primeros 15 bytes (area de clave)
- Todas las regiones no-vacias del registro con offsets, hex y ASCII
- Escaneo de patrones de fecha (YYYYMMDD) en los primeros 100 registros

### 2.4 Analyzer (`siigo-sync/cmd/analyze/main.go`)
Escaner masivo que procesa los 178 archivos ISAM del directorio de datos y genera `isam_tables_v3.json` con:
- Record size, numero de registros
- Campos detectados automaticamente (fechas, NITs, texto, numeros)
- Known layouts para archivos ya mapeados

### 2.5 Peek tool (`siigo-sync/cmd/peek/main.go`)
Herramienta de validacion que ejecuta todos los parsers y muestra estadisticas de campos vacios, distribucion de tipos, y los primeros N registros de cada tabla.

---

## 3. Metodologia: Como descubrimos cada tabla

El proceso para mapear un archivo ISAM desconocido sigue estos pasos:

### Paso 1: Identificar el archivo y su tamano de registro

```
$ ls -la C:\SIIWI02\Z17*
Z17      (datos, ~104KB)
Z17.idx  (indices)
```

El header del archivo (offset 0x38, big-endian 16-bit) contiene el tamano del registro:
```go
recSize := binary.BigEndian.Uint16(data[0x38:0x3A])
// Z17: recSize = 1438 bytes
```

### Paso 2: Leer registros con EXTFH y hacer hex dump

Ejecutamos el hexdump tool apuntando al archivo:
```
$ go run ./cmd/hexdump/ C:\SIIWI02\Z17
```

Esto muestra cada registro con sus regiones de datos. Ejemplo del primer registro Z17:
```
=== Record 1 (len=1438) first byte='G' ===
  KEY: 47 30 30 31 30 30 30 30 30 30 30 30 30 30 32  |G00100000000002|
  [0000-0048] (49 bytes) hex: 47 30 30 31 ... 53 55 50 45 52 4d 45 52 43 41 44 4f 53
                               G  0  0  1  ... S  U  P  E  R  M  E  R  C  A  D  O  S
```

### Paso 3: Identificar campos por inspeccion visual

Buscamos patrones reconocibles en los datos:

1. **Fechas**: Patron `20YYMMDD` o `19YYMMDD` — 8 digitos consecutivos
2. **Codigos de empresa**: `001`, `002` — 3 digitos al inicio
3. **NITs**: 6-13 digitos numericos
4. **Nombres**: Cadenas ASCII mayusculas con espacios
5. **Cuentas PUC**: 13 digitos que empiezan con 1-9 (plan contable colombiano)
6. **Tipos D/C**: Un solo byte `D` (debito) o `C` (credito)
7. **Tipo de registro**: Un solo byte letra al inicio (G, L, N, R, F, P, etc.)

### Paso 4: Mapear offsets escribiendo un diagnostico

Creamos un script temporal que muestra bytes en posiciones candidatas para multiples registros:

```go
for i, rec := range records {
    fmt.Printf("[18:20] Campo18 = %q  (hex: %02x %02x)\n", string(rec[18:20]), rec[18], rec[19])
    fmt.Printf("[36:76] Nombre  = %q\n", strings.TrimSpace(string(rec[36:76])))
}
```

Comparamos los valores extraidos contra datos conocidos de Siigo para validar que los offsets son correctos.

### Paso 5: Validar con multiples registros

Un offset correcto debe producir datos coherentes en TODOS los registros, no solo en uno. Por ejemplo:
- Si offset 28 da fechas validas (2012-2016) en los 73 registros de Z17 → correcto
- Si offset 38 da nombres truncados ("PERMERCADOS" en vez de "SUPERMERCADOS") → incorrecto, probar offset 36

### Paso 6: Construir el parser y correr peek

Una vez mapeados los offsets, escribimos el parser en `siigo-common/parsers/` y validamos con peek:
```
$ go run ./cmd/peek/
=== TERCEROS (Z17) - DETALLE ===
Total terceros: 72
Clientes (G): 2
Tipos doc: map[11:14 13:1 14:2 17:8 25:1 41:1 51:43 61:2]
```

---

## 4. EXTFH vs Binary Reader: La discrepancia de offsets

**Descubrimiento critico**: Los registros leidos con EXTFH y los leidos con el binary reader tienen offsets DIFERENTES para los mismos campos.

| Metodo | Marcadores | Offset nombre (Z17) | Offset fecha (Z17) |
|--------|-----------|---------------------|---------------------|
| EXTFH | Sin marcadores (datos limpios) | 36 | 28 |
| Binary | Incluye 2 bytes de marcador al inicio | 43 | 34 |

**Causa**: El binary reader incluye los 2 bytes del marcador de registro (status nibble + size) como parte del registro. EXTFH los elimina y entrega solo los datos.

**Solucion**: Todos los parsers detectan que lector se uso con `isam.ExtfhAvailable()` y aplican offsets diferentes:

```go
if extfh {
    nombre = isam.ExtractField(rec, 36, 40)  // EXTFH: datos limpios
} else {
    nombre = isam.ExtractField(rec, 43, 40)  // Binary: +5 offset por marcadores
}
```

---

## 5. Tablas parseadas: Estructura verificada campo por campo

### 5.1 Z17 — Terceros (Clientes/Proveedores)

**Archivo**: `C:\SIIWI02\Z17`
**Record size**: 1,438 bytes
**Total registros**: 73 (empresa demo)
**Parser**: `siigo-common/parsers/terceros.go`

#### Estructura EXTFH (verificada con hex dump):

```
Offset  Largo  Campo           Ejemplo                           Notas
------  -----  --------------- --------------------------------  -------------------------
0       1      TipoClave       G                                 G=general, L=linea, N=NIT, R=referencia
1       3      Empresa         001                               Codigo de empresa
4       14     Codigo          00000000002001                    Codigo interno del tercero
18      2      TipoDoc         13                                13=NIT, 11=CC, 14=NIT-ext, 41=PAS
20      2      SubTipo         30                                Subtipo de cuenta (no usado en sync)
22      6      NumeroDoc       050000                            Numero de documento (NIT o CC)
28      8      FechaCreacion   20121030                          YYYYMMDD
36      40     Nombre          SUPERMERCADOS LA GRAN ESTRELLA    Razon social o nombre
76-85   10     (padding)       espacios                          No contiene datos
86      1      TipoCtaPref     C                                 D=debito, C=credito
87+     ~1351  (datos extra)   ceros, packed decimal, config     Datos contables adicionales
```

#### Tipos de clave y su significado:

| TipoClave | Cantidad | Descripcion |
|-----------|----------|-------------|
| G | 2 | Registro maestro del tercero (datos principales) |
| L | 35 | Linea de configuracion contable (cuentas asociadas) |
| N | 33 | Registro de acceso por NIT/documento |
| R | 2 | Referencia o relacion |

#### Tipos de documento:

| TipoDoc | Significado | Cantidad |
|---------|-------------|----------|
| 13 | NIT (Numero de Identificacion Tributaria) | 1 |
| 11 | CC (Cedula de Ciudadania) | 14 |
| 14 | NIT extranjero | 2 |
| 41 | Pasaporte | 1 |
| 51 | Codigo interno de cuenta | 43 |
| 17 | Codigo contable contrapartida | 8 |
| 25 | Codigo de pago | 1 |
| 61 | Otro | 2 |

#### Correccion aplicada (2026-03-07):

| Campo | Offset anterior (incorrecto) | Offset corregido | Evidencia |
|-------|------------------------------|-------------------|-----------|
| TipoDoc | 20 (daba "30", "10", "05") | 18 (da "13"=NIT, "11"=CC) | Coincide con codigos DIAN colombianos |
| Nombre | 38 (cortaba primeras 2 letras) | 36 (nombres completos) | "PERMERCADOS"→"SUPERMERCADOS", "GURO"→"SEGURO" |
| TipoTercero | 36 (capturaba inicio del nombre) | Eliminado del struct | "SU", "SA", "SE" eran letras del nombre |

**Evidencia de la correccion del nombre** (antes vs despues):

| Antes (offset 38) | Despues (offset 36) |
|--------------------|---------------------|
| PERMERCADOS LA GRAN ESTRELLA | SUPERMERCADOS LA GRAN ESTRELLA |
| ELDOS | SUELDOS |
| LARIO INTEGRAL | SALARIO INTEGRAL |
| LARIOS POR PAGAR | SALARIOS POR PAGAR |
| GURO CONTRA INCENDIO | SEGURO CONTRA INCENDIO |
| OTA MANTENIMIENTO SOFTWARE | CUOTA MANTENIMIENTO SOFTWARE |
| GUROS GENERALES | SEGUROS GENERALES |

---

### 5.2 Z06CP — Productos

**Archivo**: `C:\SIIWI02\Z06CP`
**Record size**: 2,036 bytes
**Total registros**: 17 (empresa demo)
**Parser**: `siigo-common/parsers/productos.go`

#### Estructura EXTFH (verificada):

```
Offset  Largo  Campo             Ejemplo                          Notas
------  -----  ----------------  -------------------------------  -------------------------
0       5      Comprobante       00001                            Numero de comprobante, zero-padded
5       5      Secuencia         00001                            Secuencia dentro del comprobante
10      1      TipoTercero       R                                R=recibo, G=general, L=linea
11      3      Grupo             001                              Grupo de inventario
14      13     CuentaContable    0000000000100                    Cuenta PUC (13 chars)
27      11     (datos numericos) packed decimal                   Valores monetarios
38      8      Fecha             20121031                         YYYYMMDD
46      40     Nombre            CAMISA POLO RALPH LAUREN HOMBRE  Descripcion del producto (40 chars)
86      6      (padding)
92      1      TipoMov           D                                D=debito, C=credito (raro en productos)
```

#### Datos de muestra:

```
1-1 | tipo:R grupo:001 | DE CLIENTES
1-2 | tipo:R grupo:001 | ALMACENES EXITO
5-1 | tipo:L grupo:999 | CAMISA POLO RALPH LAUREN HOMBRE T-M
5-2 | tipo:L grupo:999 | PORTATIL TOSHIBA DYNABOOK NX
5-3 | tipo:L grupo:999 | CUENTA PUENTE
6-1 | tipo:L grupo:010 | AJUSTE PREDETERMINADO
```

---

### 5.3 Z49 — Movimientos / Transacciones

**Archivo**: `C:\SIIWI02\Z49`
**Record size**: 2,295 bytes
**Total registros**: 3,233 (parseados)
**Parser**: `siigo-common/parsers/movimientos.go`

#### Estructura EXTFH (verificada):

```
Offset  Largo  Campo              Ejemplo                          Notas
------  -----  -----------------  -------------------------------  ----------------------------
0       1      TipoComprobante    R                                Letra: R=recibo, F=factura, etc.
1       3      CodigoComprobante  001                              Codigo numerico (001, 010, 100)
4       11     NumeroDoc          00000006976                      Numero de documento, zero-padded
15      35     NombreTercero      SANDRA CORONEL                   Solo presente en algunos tipos
50+     var    (datos binarios)   packed decimal                   Montos, no decodificados como texto
72+     var    Descripcion        ABONO A FACT. CTA 01.            Texto libre, posicion variable
```

**Importante**: Z49 via EXTFH **NO contiene** fechas, cuentas contables, valores monetarios como texto. Estos datos estan en formato packed decimal o se obtienen de Z03 (movimientos contables) y Z09 (cartera).

#### Mapeo de letra tipo a codigo Siigo:

| Byte | Codigo | Tipo comprobante | Cantidad |
|------|--------|------------------|----------|
| R | RC | Recibo de Caja | 452 |
| F | FV | Factura de Venta | 148 |
| T | TR | Traslado | 1,566 |
| N | ND | Nota Debito | 522 |
| E | CE | Comprobante de Egreso | 44 |
| P | PG | Pago | 62 |
| C | CC | Comprobante de Contabilidad | 8 |
| H | HJ | Hoja de Trabajo | 53 |
| J | JR | Journal | 131 |
| L | LB | Libro | 219 |

#### Registros tipo espacio (0x20):
Los registros con espacio al inicio son indexados por NIT, no por tipo de comprobante. El campo NumeroDoc contiene el NIT del tercero.

---

### 5.4 Z09YYYY — Cartera (Cuentas por Cobrar/Pagar)

**Archivo**: `C:\SIIWI02\Z092013`, `Z092014`, `Z092015`, `Z092016`
**Record size**: 1,152 bytes
**Total registros**: 567 (2013), 115 (2014), 77 (2016)
**Parser**: `siigo-common/parsers/cartera.go`

#### Estructura EXTFH (verificada):

```
Offset  Largo  Campo             Ejemplo                         Notas
------  -----  ----------------  ------------------------------  -------------------------
0       1      TipoRegistro      F                               F=factura, G=general, L=libro, P=pago, R=recibo
1       3      Empresa           001                             Codigo de empresa
4       5      (padding)         nulls (0x00)                    Separador
9       1      (separador)       0x1F                            Unit separator
10      5      Secuencia         00001                           Numero secuencial del asiento
15      1      TipoDoc           N                               N=NIT, etc.
16      13     NitTercero        0000900019401                   NIT zero-padded (13 chars)
29      13     CuentaContable    0004135050100                   Cuenta PUC (13 chars)
42      8      Fecha             20130131                        YYYYMMDD
50      43     (packed decimal)  datos binarios                  Montos (no decodificados)
93      40     Descripcion       PORTATIL TOSHIBA DYNABOOK NX    Texto de descripcion
133     10     (padding)
143     1      TipoMov           D                               D=debito, C=credito
```

#### Tipos de registro y distribucion (Z092013):

| Tipo | Significado | Cantidad | Debitos | Creditos |
|------|-------------|----------|---------|----------|
| F | Factura | 183 | 95 | 88 |
| P | Pago | 110 | 53 | 57 |
| L | Libro/contabilidad | 181 | 91 | 90 |
| R | Recibo | 50 | 22 | 28 |
| G | General | 43 | 33 | 10 |
| **Total** | | **567** | **294** | **273** |

#### Datos de muestra:

```
[F] nit:900019401  fecha:2013-01-31 mov:C cuenta:0004135050100 | PORTATIL TOSHIBA DYNABOOK NX
[P] nit:800200300  fecha:2013-01-31 mov:D cuenta:0001435050100 | MOUSE TOSHIB MICROTRAVELER 900S
[R] nit:900019401  fecha:2013-01-31 mov:C cuenta:0001305050100 | ABONA FACTURA
[G] nit:52850223   fecha:2013-01-31 mov:D cuenta:0002205050100 | CANCELA FACTURA
[L] nit:800100200  fecha:2013-01-31 mov:D cuenta:0005105060100 | SUELDOS
```

---

### 5.5 Z06 — Maestros de Configuracion

**Archivo**: `C:\SIIWI02\Z06`
**Record size**: 4,096 bytes
**Parser**: `siigo-common/parsers/maestros.go`

#### Estructura EXTFH (verificada):

```
Offset  Largo  Campo         Ejemplo                    Notas
------  -----  ------------  -------------------------  -------------------------
0       1      Tipo          A                          Letra de tipo de maestro
2       7      Codigo        0001000                    Codigo de 7 digitos
31      20     Nombre        SUCURSAL PRINCIPAL         Nombre/descripcion
70      20     Responsable   JUAN PEREZ                 Solo para tipos A/B
90      30     Direccion     CRA 15 #45-20              Solo para tipos A/B
200     50     (zona email)  correo@empresa.com         Se busca @ para extraer
```

#### Tipos de maestro en Z06:

| Tipo | Significado | Ejemplo |
|------|-------------|---------|
| A | Sucursales / Centros de costo | SUCURSAL PRINCIPAL |
| B | Bodegas | BODEGA PRINCIPAL |
| C | Conceptos de nomina | SALARIO BASICO |
| I | Grupos de inventario | GRUPO GENERAL |
| V | Vendedores | VENDEDOR 01 |
| X | Actividades economicas | COMERCIO AL POR MENOR |
| Z | Zonas | ZONA NORTE |
| L | Lineas de activos fijos | EQUIPOS DE COMPUTO |
| g | Ciudades | BOGOTA |
| k | Paises | COLOMBIA |
| d | Calificaciones | EXCELENTE |
| p | Formas de pago | EFECTIVO |
| M | Motivos de cobro | VENCIMIENTO |
| O | Monedas | DOLAR USD |
| T | Tipos de empresa | PERSONA JURIDICA |
| Y | Seguros | SEGURO TODO RIESGO |

---

## 6. Errores comunes y como los detectamos

### 6.1 Nombre truncado (Z17)

**Sintoma**: Los nombres aparecian sin las primeras 2 letras.
**Causa**: El offset del nombre estaba en 38 en vez de 36.
**Como lo detectamos**: Multiples nombres mostraban patrones de truncamiento consistente (siempre 2 caracteres):
- "PERMERCADOS" → falta "SU" (SUPERMERCADOS)
- "GURO CONTRA INCENDIO" → falta "SE" (SEGURO)

**Metodo de verificacion**: Hex dump mostrando bytes 36-48 del registro:
```
Pos 36: 53 55 50 45 52 4d ...
         S  U  P  E  R  M ...
```
Confirmado: el nombre empieza en byte 36.

### 6.2 TipoDoc incorrecto (Z17)

**Sintoma**: Los tipos de documento mostraban valores no estandar (05, 10, 30, 65).
**Causa**: Se leia de offset 20 en vez de 18.
**Como lo detectamos**: Al leer offset 18, los valores cambiaron a codigos DIAN validos:
- 13 = NIT (correcto para un proveedor)
- 11 = CC (correcto para una persona natural)

### 6.3 Campo fantasma TipoTercero (Z17)

**Sintoma**: El campo "TipoTercero" mostraba valores como "SU", "SA", "SE", "CU".
**Causa**: El offset 36:38 (donde se leia TipoTercero) era en realidad el inicio del nombre.
**Como lo detectamos**: Los valores coincidian exactamente con las primeras 2 letras de cada nombre truncado.

### 6.4 Offsets EXTFH vs Binary

**Sintoma**: Parser funcionaba con EXTFH pero no con binary reader (o viceversa).
**Causa**: Binary reader incluye 2 bytes de marcador al inicio del registro.
**Solucion**: Dual-mode con `isam.ExtfhAvailable()`.

---

## 7. Herramientas de diagnostico: Como usarlas

### Hex dump de un archivo ISAM

```bash
cd siigo-sync
go run ./cmd/hexdump/ C:\SIIWI02\Z17
```

Muestra regiones no-vacias de registros seleccionados + escaneo de fechas.

### Diagnostico de offsets especificos

Crear un script temporal en Go que imprima posiciones candidatas:

```go
for i, rec := range records[:15] {
    fmt.Printf("Record %d:\n", i)
    fmt.Printf("  [18:20] = %q\n", string(rec[18:20]))
    fmt.Printf("  [36:76] = %q\n", strings.TrimSpace(string(rec[36:76])))
}
```

### Validacion completa con peek

```bash
cd siigo-sync
go run ./cmd/peek/
```

Muestra estadisticas de campos vacios, distribucion de tipos, y primeros 10 registros de cada parser.

### Scan masivo con analyzer

```bash
cd siigo-sync
go run ./cmd/analyze/
```

Genera `isam_tables_v3.json` con estructura detectada de los 178 archivos ISAM.

---

## 8. Archivos del proyecto

| Archivo | Ubicacion | Funcion |
|---------|-----------|---------|
| `extfh.go` | `siigo-common/isam/` | Wrapper EXTFH (lectura via DLL) |
| `reader.go` | `siigo-common/isam/` | Binary reader fallback |
| `terceros.go` | `siigo-common/parsers/` | Parser Z17 (clientes/proveedores) |
| `productos.go` | `siigo-common/parsers/` | Parser Z06CP (productos) |
| `movimientos.go` | `siigo-common/parsers/` | Parser Z49 (movimientos) |
| `cartera.go` | `siigo-common/parsers/` | Parser Z09YYYY (cartera) |
| `maestros.go` | `siigo-common/parsers/` | Parser Z06 (config maestros) |
| `main.go` | `siigo-sync/cmd/hexdump/` | Herramienta de hex dump |
| `main.go` | `siigo-sync/cmd/peek/` | Herramienta de validacion |
| `main.go` | `siigo-sync/cmd/analyze/` | Escaner masivo de tablas |

---

## 9. Resumen de offsets verificados (EXTFH mode)

### Z17 — Terceros
| Offset | Largo | Campo | Verificado |
|--------|-------|-------|------------|
| 0 | 1 | TipoClave (G/L/N/R) | Si |
| 1 | 3 | Empresa | Si |
| 4 | 14 | Codigo | Si |
| 18 | 2 | TipoDoc (13=NIT, 11=CC) | Si (corregido de offset 20) |
| 22 | 6 | NumeroDoc | Si |
| 28 | 8 | FechaCreacion (YYYYMMDD) | Si (confirmado en 73/73 registros) |
| 36 | 40 | Nombre | Si (corregido de offset 38) |
| 86 | 1 | TipoCtaPref (D/C) | Si |

### Z06CP — Productos
| Offset | Largo | Campo | Verificado |
|--------|-------|-------|------------|
| 0 | 5 | Comprobante | Si |
| 5 | 5 | Secuencia | Si |
| 10 | 1 | TipoTercero (R/G/L) | Si |
| 11 | 3 | Grupo | Si |
| 14 | 13 | CuentaContable | Si |
| 38 | 8 | Fecha (YYYYMMDD) | Si |
| 46 | 40 | Nombre | Si |
| 92 | 1 | TipoMov (D/C) | Si |

### Z49 — Movimientos
| Offset | Largo | Campo | Verificado |
|--------|-------|-------|------------|
| 0 | 1 | TipoComprobante (letra) | Si |
| 1 | 3 | CodigoComprobante | Si |
| 4 | 11 | NumeroDoc | Si |
| 15 | 35 | NombreTercero | Parcial (solo algunos tipos) |
| 72+ | var | Descripcion | Si (via findDescripcion) |

### Z09YYYY — Cartera
| Offset | Largo | Campo | Verificado |
|--------|-------|-------|------------|
| 0 | 1 | TipoRegistro (F/G/L/P/R) | Si |
| 1 | 3 | Empresa | Si |
| 10 | 5 | Secuencia | Si |
| 15 | 1 | TipoDoc | Si |
| 16 | 13 | NitTercero | Si |
| 29 | 13 | CuentaContable | Si |
| 42 | 8 | Fecha (YYYYMMDD) | Si |
| 93 | 40 | Descripcion | Si |
| 143 | 1 | TipoMov (D/C) | Si |

### Z06 — Maestros
| Offset | Largo | Campo | Verificado |
|--------|-------|-------|------------|
| 0 | 1 | Tipo (A/B/I/V/etc.) | Si |
| 2 | 7 | Codigo | Si |
| 31 | 20 | Nombre | Si |
| 70 | 20 | Responsable (A/B) | Si |
| 90 | 30 | Direccion (A/B) | Si |
| 200 | 50 | Zona email (A/B) | Parcial |

---

## 10. Campos pendientes por decodificar

| Tabla | Zona | Bytes | Contenido probable |
|-------|------|-------|--------------------|
| Z17 | 87-1437 | ~1350 | Config contable, limites credito, packed decimal |
| Z49 | 50-71 | ~20 | Montos en packed decimal COBOL |
| Z09 | 50-92 | ~42 | Montos debito/credito en packed decimal |
| Z06CP | 27-37 | ~10 | Precios/cantidades en packed decimal |
| Z06 | 120-4095 | ~3975 | Config adicional por tipo de maestro |

Los campos monetarios estan en **packed decimal COBOL** (BCD - Binary Coded Decimal) donde cada nibble (4 bits) representa un digito, y el ultimo nibble indica el signo (0xC=positivo, 0xD=negativo, 0xF=unsigned). Ejemplo: `0x01 0x50 0x00 0x0C` = +15000.

---

## 11. Arquitectura del modulo compartido (siigo-common)

### 11.1 Estructura de directorios

```
siigo/
├── siigo-common/          # Modulo Go compartido (fuente unica de verdad)
│   ├── go.mod             # module siigo-common
│   ├── go.sum
│   ├── isam/
│   │   ├── extfh.go       # Wrapper EXTFH (DLL syscall) + ReadIsamFile()
│   │   └── reader.go      # Binary reader fallback + ExtractField()
│   └── parsers/
│       ├── terceros.go    # Z17
│       ├── productos.go   # Z06CP
│       ├── movimientos.go # Z49
│       ├── cartera.go     # Z09YYYY
│       └── maestros.go    # Z06
│
├── siigo-sync/            # CLI de sincronizacion
│   ├── go.mod             # require siigo-common v0.0.0 + replace => ../siigo-common
│   ├── cmd/
│   │   ├── hexdump/       # Herramienta de hex dump
│   │   ├── peek/          # Herramienta de validacion de parsers
│   │   ├── analyze/       # Escaner masivo de 178 tablas
│   │   └── scan/          # Otros utilitarios
│   ├── sync/              # Logica de deteccion de cambios
│   ├── api/               # Cliente HTTP hacia Finearom
│   └── config/            # Configuracion JSON
│
├── siigo-app/             # App de escritorio (Wails + Go)
│   ├── go.mod             # require siigo-common v0.0.0 + replace => ../siigo-common
│   └── ...
│
└── docs/                  # Documentacion (este archivo)
```

### 11.2 Dependencia entre modulos

Ambos proyectos (siigo-sync y siigo-app) importan siigo-common usando `replace` directive local:

**siigo-sync/go.mod:**
```
module siigo-sync
go 1.24.0
require siigo-common v0.0.0
replace siigo-common => ../siigo-common
```

**siigo-app/go.mod:**
```
module siigo-app
go 1.24.0
require siigo-common v0.0.0
replace siigo-common => ../siigo-common
```

**siigo-common/go.mod:**
```
module siigo-common
go 1.24.0
require golang.org/x/text v0.34.0
```

**Regla critica**: NUNCA duplicar los paquetes `isam/` ni `parsers/` en siigo-sync o siigo-app. Siempre importar de `siigo-common`.

### 11.3 Dependencia externa unica

El unico paquete externo que necesita siigo-common es `golang.org/x/text` para decodificar Windows-1252 a UTF-8:

```go
import "golang.org/x/text/encoding/charmap"

decoder := charmap.Windows1252.NewDecoder()
utf8bytes, _ := decoder.Bytes(rawBytes)
```

---

## 12. API del paquete isam (referencia completa)

### 12.1 Funciones principales

```go
package isam

// ReadIsamFile lee todos los registros de un archivo ISAM.
// Intenta EXTFH primero, cae a binary reader si la DLL no esta disponible.
// Retorna: registros ([][]byte), tamaño de registro (int), error.
func ReadIsamFile(path string) ([][]byte, int, error)

// ReadIsamFileWithMeta lee registros + metadatos detallados (numKeys, format, etc.)
func ReadIsamFileWithMeta(path string) ([][]byte, *IsamFileMeta, error)

// ExtfhAvailable retorna true si la DLL cblrtsm.dll esta cargada.
// Usar para decidir que offsets aplicar en los parsers.
func ExtfhAvailable() bool

// ExtractField extrae un campo de texto de longitud fija.
// Recorta espacios y nulls al final. Decodifica Windows-1252 a UTF-8.
// Parametros: rec=registro, offset=posicion inicial, length=largo del campo.
func ExtractField(rec []byte, offset, length int) string

// ExtractNumericField extrae un campo numerico y recorta ceros a la izquierda.
func ExtractNumericField(rec []byte, offset, length int) string

// DecodeText decodifica bytes Windows-1252 a string UTF-8.
func DecodeText(data []byte) string

// GetModTime retorna el timestamp de modificacion de un archivo (para deteccion de cambios).
func GetModTime(path string) (int64, error)
```

### 12.2 Structs del binary reader

```go
// IsamHeader contiene metadatos del header del archivo ISAM (primeros 1024 bytes).
type IsamHeader struct {
    Magic           uint16 // 0x33FE (standard) o 0x30xx (variante)
    RecordSize      int    // Tamano del registro desde offset 0x38
    ExpectedRecords int    // Conteo esperado desde offset 0x40
    HasIndex        bool   // True si existe archivo .idx
    IsValid         bool   // True si el magic es reconocido
}

// IsamFileMeta contiene metadatos completos (retornado por ReadIsamFileWithMeta).
type IsamFileMeta struct {
    RecSize         int
    RecordCount     int
    NumKeys         int       // Numero de indices/claves
    Format          int       // Formato ISAM (IDXFORMAT)
    Keys            []KeyInfo // Info de cada clave
    HasIndex        bool
    UsedEXTFH       bool
    DLLPath         string
}
```

### 12.3 Como funciona ExtractField (critico para parsers)

```go
func ExtractField(rec []byte, offset, length int) string {
    // 1. Si offset >= len(rec), retorna ""
    // 2. Extrae rec[offset:offset+length]
    // 3. Recorta trailing spaces (0x20) y nulls (0x00)
    // 4. Decodifica de Windows-1252 a UTF-8
    // 5. Retorna string limpio
}
```

**Ejemplo de uso:**
```go
// Extraer nombre de 40 caracteres desde offset 36
nombre := isam.ExtractField(rec, 36, 40)
// Input bytes:  "SUPERMERCADOS LA GRAN ESTRELLA          " (40 bytes)
// Output:       "SUPERMERCADOS LA GRAN ESTRELLA" (sin trailing spaces)
```

**Nota**: ExtractField NO recorta espacios al inicio (leading spaces). Si necesitas eso, usa `strings.TrimSpace()` o `strings.TrimLeft()` sobre el resultado.

---

## 13. Guia paso a paso: Como crear un nuevo parser

Esta guia es para agregar soporte de un nuevo archivo ISAM (ej: Z70, Z003, C03, etc.).

### Paso 1: Verificar que el archivo existe y es legible

```bash
# Ver archivos disponibles
ls C:/SIIWI02/Z70*

# Ver tamano
ls -la C:/SIIWI02/Z70
```

### Paso 2: Obtener hex dump del archivo

```bash
cd siigo-sync
go run ./cmd/hexdump/ 'C:\SIIWI02\Z70'
```

Esto produce:
- `EXTFH available: true/false`
- `Total records: N`
- Para registros seleccionados: KEY area + todas las regiones no-vacias
- Escaneo de fechas en los primeros 100 registros

**Guardar la salida** — es la materia prima para mapear campos.

### Paso 3: Escribir un script de diagnostico temporal

Crear un archivo temporal (no commitear) para explorar offsets:

```go
package main

import (
    "fmt"
    "siigo-common/isam"
    "strings"
)

func main() {
    records, recSize, err := isam.ReadIsamFile(`C:\SIIWI02\Z70`)
    if err != nil {
        fmt.Printf("ERROR: %v\n", err)
        return
    }
    fmt.Printf("EXTFH: %v, Records: %d, RecSize: %d\n\n", isam.ExtfhAvailable(), len(records), recSize)

    for i, rec := range records {
        if i >= 15 { break }
        if len(rec) < 20 { continue }

        fmt.Printf("--- Record %d (len=%d) ---\n", i, len(rec))

        // Probar diferentes rangos para encontrar campos
        // Ajustar estos offsets segun lo que muestre el hex dump
        fmt.Printf("  [0:1]   = %q  (hex: %02x)\n", string(rec[0]), rec[0])
        fmt.Printf("  [1:4]   = %q\n", strings.TrimSpace(string(rec[1:4])))

        // Buscar fechas
        for j := 0; j < len(rec)-8 && j < 200; j++ {
            if rec[j] == '2' && rec[j+1] == '0' {
                allDigit := true
                for k := j; k < j+8; k++ {
                    if rec[k] < '0' || rec[k] > '9' { allDigit = false; break }
                }
                if allDigit {
                    fmt.Printf("  Fecha en offset %d: %s\n", j, string(rec[j:j+8]))
                }
            }
        }

        // Buscar texto legible
        for j := 0; j < len(rec)-5 && j < 300; j++ {
            if rec[j] >= 'A' && rec[j] <= 'Z' {
                end := j
                for end < len(rec) && ((rec[end] >= 'A' && rec[end] <= 'Z') || rec[end] == ' ') {
                    end++
                }
                if end-j > 5 {
                    fmt.Printf("  Texto en offset %d-%d: %q\n", j, end, strings.TrimSpace(string(rec[j:end])))
                    j = end
                }
            }
        }
        fmt.Println()
    }
}
```

Ejecutar:
```bash
cd siigo-sync
go run /tmp/diagnostic.go
```

### Paso 4: Crear el parser en siigo-common/parsers/

Una vez identificados los offsets, crear el archivo del parser:

```go
// siigo-common/parsers/nuevo_parser.go
package parsers

import (
    "crypto/sha256"
    "fmt"
    "siigo-common/isam"
    "strings"
)

// NuevoRegistro representa un registro del archivo ZXXX
type NuevoRegistro struct {
    Campo1  string `json:"campo1"`
    Campo2  string `json:"campo2"`
    // ... mas campos
    Hash    string `json:"hash"`
}

// ParseNuevo lee el archivo ZXXX y retorna todos los registros
func ParseNuevo(dataPath string) ([]NuevoRegistro, error) {
    path := dataPath + "ZXXX"
    records, _, err := isam.ReadIsamFile(path)
    if err != nil {
        return nil, err
    }

    extfh := isam.ExtfhAvailable()
    var resultado []NuevoRegistro
    for _, rec := range records {
        r := parseNuevoRecord(rec, extfh)
        if r.Campo1 == "" { // filtrar registros vacios
            continue
        }
        resultado = append(resultado, r)
    }
    return resultado, nil
}

func parseNuevoRecord(rec []byte, extfh bool) NuevoRegistro {
    if len(rec) < 50 { // minimo razonable
        return NuevoRegistro{}
    }

    hash := sha256.Sum256(rec)

    if extfh {
        // Offsets verificados con hex dump (EXTFH = datos limpios)
        return NuevoRegistro{
            Campo1: isam.ExtractField(rec, 0, 10),
            Campo2: strings.TrimSpace(isam.ExtractField(rec, 10, 40)),
            Hash:   fmt.Sprintf("%x", hash[:8]),
        }
    }

    // Fallback binary: offsets diferentes (+2 o mas por marcadores)
    return NuevoRegistro{
        Campo1: isam.ExtractField(rec, 2, 10),
        Campo2: strings.TrimSpace(isam.ExtractField(rec, 12, 40)),
        Hash:   fmt.Sprintf("%x", hash[:8]),
    }
}
```

### Paso 5: Agregar al peek tool para validacion

Editar `siigo-sync/cmd/peek/main.go` y agregar una seccion para el nuevo parser:

```go
// === NUEVO ===
fmt.Println("=== NUEVO (ZXXX) - DETALLE ===")
nuevos, err := parsers.ParseNuevo(dataPath)
if err != nil {
    fmt.Printf("ERROR: %v\n", err)
}
fmt.Printf("Total: %d\n", len(nuevos))
for i, n := range nuevos {
    if i >= 10 { break }
    fmt.Printf("  [%d] campo1:%s | %s\n", i+1, n.Campo1, n.Campo2)
}
```

### Paso 6: Compilar y validar

```bash
# Compilar todo (verifica que no haya errores en ambos proyectos)
cd siigo-sync && go build ./...
cd ../siigo-app && go build ./...

# Ejecutar validacion
cd ../siigo-sync && go run ./cmd/peek/
```

### Paso 7: Verificar datos coherentes

Revisar en la salida de peek:
- [ ] Los campos no estan vacios (o estan vacios por razon valida)
- [ ] Las fechas son validas (rango 2010-2030 para datos reales)
- [ ] Los nombres no estan truncados (comparar con datos conocidos de Siigo)
- [ ] Los codigos de tipo tienen sentido (letras mayusculas, numeros conocidos)
- [ ] Los NITs son numeros validos (6-13 digitos)
- [ ] Las cuentas PUC empiezan con 1-9 y tienen 13 chars

---

## 14. Patrones de codigo recurrentes en los parsers

### 14.1 Estructura base de un parser

Todos los parsers siguen este patron:

```
1. ParseXxx(dataPath) → []Xxx, error        // Funcion publica, lee archivo completo
2. parseXxxRecord(rec, extfh) → Xxx          // Funcion privada, parsea un registro
3. parseXxxEXTFH(rec, hash) → Xxx            // Offsets para modo EXTFH
4. parseXxxHeuristic(rec, hash) → Xxx        // Fallback con escaneo inteligente
5. ToFinearomXxx() → map[string]interface{}  // Conversion para API Finearom
```

### 14.2 Hash para deteccion de cambios

Cada parser genera un hash SHA256 truncado a 8 bytes para detectar cambios:

```go
hash := sha256.Sum256(rec)
hashStr := fmt.Sprintf("%x", hash[:8])  // 16 chars hex
```

Este hash se usa en el sync service para saber si un registro cambio desde la ultima sincronizacion.

### 14.3 Funcion findDescripcion (busca texto legible)

Cuando no se sabe la posicion exacta de un campo de texto, se usa `findDescripcion()`:

```go
// Busca la secuencia mas larga de texto legible (A-Z, 0-9, espacios, puntuacion)
// a partir de startFrom, hasta un maximo de 500 bytes.
// Retorna "" si no encuentra texto de mas de 5 caracteres.
func findDescripcion(rec []byte, startFrom int) string
```

Esta funcion es util para el fallback heuristico y para campos de posicion variable (como la descripcion en Z49).

### 14.4 Funciones de validacion

```go
// Valida que un string de 8 chars sea una fecha YYYYMMDD valida
func looksLikeDate(s string) bool

// Valida que un string parezca una cuenta PUC (empieza con 1-9)
func looksLikeAccount(s string) bool

// Verifica que N bytes consecutivos sean digitos
func isDigitRange(rec []byte, start, length int) bool
```

### 14.5 Conversion a formato Finearom

Cada struct tiene un metodo `ToFinearomXxx()` que genera un map listo para enviar como JSON a la API:

```go
func (t *Tercero) ToFinearomClient() map[string]interface{} {
    nit := strings.TrimLeft(t.NumeroDoc, "0")
    return map[string]interface{}{
        "nit":             nit,
        "client_name":     t.Nombre,
        "business_name":   t.Nombre,
        "taxpayer_type":   tipoDoc,  // Mapeado de codigo a descripcion
        "siigo_codigo":    t.Codigo,
        "siigo_empresa":   t.Empresa,
        "siigo_sync_hash": t.Hash,
    }
}
```

---

## 15. Directorio de datos de Siigo

### 15.1 Ubicacion

El directorio de datos se configura en `C:\Siigo\FILEPATH.TXT` y tipicamente es:
```
C:\SIIWI02\
```

### 15.2 Configuracion EXTFH

El runtime COBOL necesita estas variables de entorno (configuradas automaticamente por `extfh.go`):

```
COBCONFIG=C:\Siigo\COBCONFIG.CFG    # Configuracion del runtime COBOL
COBOPT=+A                            # Opciones del runtime
PATH=...;C:\Siigo                    # Para encontrar cblrtsm.dll
```

El archivo `COBCONFIG.CFG` apunta al file handler:
```ini
[EXTFH]
config-file=C:\Siigo\EXTFH.CFG
```

Y `EXTFH.CFG` configura el acceso a archivos ISAM:
```ini
[XFH-DEFAULT]
IGNORELOCK=ON
IDXFORMAT=8
```

### 15.3 Archivos ISAM disponibles (principales)

| Archivo | RecSize | Registros | Descripcion | Parser |
|---------|---------|-----------|-------------|--------|
| Z17 | 1,438 | 73 | Terceros (clientes/proveedores) | terceros.go |
| Z06 | 4,096 | ~585 | Maestros de configuracion | maestros.go |
| Z06CP | 2,036 | 17 | Productos/items reales | productos.go |
| Z49 | 2,295 | 3,235 | Movimientos/transacciones | movimientos.go |
| Z49YYYY | 2,295 | var | Movimientos por ano | movimientos.go |
| Z09YYYY | 1,152 | var | Cartera (CxC/CxP) | cartera.go |
| Z03YYYY | 1,152 | ~2,851 | Movimientos contables | (pendiente) |
| Z04YYYY | 3,520 | ~125 | Detalle de movimientos | (pendiente) |
| Z003 | 61 | 1+ | Usuarios del sistema | (pendiente) |
| C03 | 512 | 17 | Plan de cuentas PUC | (pendiente) |
| ZDANE | 256 | 1,384 | Ciudades de Colombia | (pendiente) |
| Z70 | ~300 | 4 | Agenda de cobro | (pendiente) |
| Z001 | ~3,080 | 1 | Datos de la empresa | (pendiente) |
| Z90ES | 250 | 1,824 | Modulos y permisos | (pendiente) |
| Z120 | 256 | 2,139 | Empleados | (pendiente) |

---

## 16. Checklist para una IA: parsear un nuevo archivo ISAM

Si eres una IA y necesitas agregar soporte para un nuevo archivo ISAM de Siigo, sigue esta checklist:

### Pre-requisitos
- [ ] El directorio de datos es `C:\SIIWI02\` (verificar en config o FILEPATH.TXT)
- [ ] El modulo compartido esta en `siigo-common/` con paquetes `isam/` y `parsers/`
- [ ] Ambos proyectos usan `replace siigo-common => ../siigo-common` en go.mod

### Descubrimiento
- [ ] Verificar que el archivo existe: `ls C:/SIIWI02/ZXXX*`
- [ ] Ejecutar hex dump: `cd siigo-sync && go run ./cmd/hexdump/ 'C:\SIIWI02\ZXXX'`
- [ ] Anotar: EXTFH available (true/false), total records, record size
- [ ] Identificar el primer byte de cada registro (tipo/clave)
- [ ] Buscar fechas en la salida (patron YYYYMMDD con offsets)
- [ ] Buscar texto legible (nombres, descripciones)
- [ ] Buscar NITs (secuencias de 6-13 digitos)
- [ ] Buscar cuentas PUC (13 digitos empezando con 1-9)
- [ ] Buscar indicadores D/C (byte individual 'D' o 'C')

### Verificacion de offsets
- [ ] Escribir script de diagnostico temporal que muestre bytes en posiciones candidatas
- [ ] Ejecutar contra 15+ registros
- [ ] Validar que los campos son coherentes en TODOS los registros
- [ ] Si un nombre aparece truncado, probar offset -1, -2
- [ ] Si un tipo_doc no coincide con codigos DIAN, probar offset -2, +2
- [ ] Fechas deben estar en rango 2010-2030 para la empresa demo

### Construccion del parser
- [ ] Crear archivo en `siigo-common/parsers/nombre.go`
- [ ] Seguir el patron: struct + ParseXxx() + parseXxxRecord() + parseXxxEXTFH()
- [ ] Incluir rama EXTFH y rama binary con offsets diferentes
- [ ] Agregar Hash SHA256 truncado a 8 bytes
- [ ] Agregar metodo ToFinearomXxx() si se va a sincronizar

### Validacion
- [ ] Compilar: `cd siigo-sync && go build ./...`
- [ ] Compilar: `cd siigo-app && go build ./...`
- [ ] Agregar al peek tool y ejecutar: `go run ./cmd/peek/`
- [ ] Verificar: cero campos vacios inesperados
- [ ] Verificar: nombres completos (no truncados)
- [ ] Verificar: fechas validas
- [ ] Verificar: tipos/codigos reconocibles

### Post-validacion
- [ ] Actualizar este documento (seccion 5 + seccion 9) con los nuevos offsets
- [ ] Actualizar MEMORY.md si es un parser de sincronizacion
- [ ] Actualizar la tabla de sincronizacion en MEMORY.md

---

## 17. Flujo completo de sincronizacion: Archivo ISAM → Finearom API

### 17.1 Diagrama del flujo

```
┌──────────────┐    ReadIsamFile()    ┌──────────────┐   ParseXxx()    ┌──────────────┐
│  Archivo     │ ──────────────────→  │  [][]byte    │ ──────────────→ │  []Struct    │
│  ISAM        │    (extfh.go o       │  registros   │   (parsers/)    │  tipados     │
│  C:\SIIWI02\ │     reader.go)       │  crudos      │                 │              │
└──────────────┘                      └──────────────┘                 └──────┬───────┘
                                                                              │
                                                                              │ DetectChanges()
                                                                              │ (sync/detector.go)
                                                                              ▼
┌──────────────┐   HTTP POST JSON    ┌──────────────┐  ToFinearomXxx() ┌──────────────┐
│  Finearom    │ ◀────────────────── │  api/client   │ ◀────────────── │  []Change    │
│  Laravel API │    (api/client.go)  │  .SyncXxx()   │                 │  new/updated │
└──────────────┘                     └──────────────┘                  └──────────────┘
```

### 17.2 Archivos que se deben parsear y como se sincronizan

#### CLIENTES — Prioridad ALTA

```
Archivo ISAM:    Z17
Parser:          parsers.ParseTercerosClientes(dataPath) → []Tercero (solo tipo G)
Detector:        sync.detectTercerosChanges() — clave: TipoClave-Empresa-Codigo
Conversion:      Tercero.ToFinearomClient() → map[string]interface{}
API Client:      api.Client.SyncClient(data)
Endpoint HTTP:   POST /siigo/clients
Clave de cruce:  NIT (campo NumeroDoc, sin ceros a la izquierda)
```

**Campos que se envian a Finearom:**
```json
{
  "nit":             "900019401",
  "client_name":     "SUPERMERCADOS LA GRAN ESTRELLA",
  "business_name":   "SUPERMERCADOS LA GRAN ESTRELLA",
  "taxpayer_type":   "CC",
  "siigo_codigo":    "00000000002002",
  "siigo_empresa":   "001",
  "siigo_sync_hash": "a1b2c3d4e5f6g7h8"
}
```

**Campos ISAM usados:**
| Campo Finearom | Campo Tercero | Offset EXTFH | Transformacion |
|----------------|---------------|-------------|----------------|
| nit | NumeroDoc | [22:28] | `strings.TrimLeft(v, "0")` |
| client_name | Nombre | [36:76] | directo |
| taxpayer_type | TipoDoc | [18:20] | mapeo: 13→NIT, 11→CC, 41→PAS |
| siigo_codigo | Codigo | [4:18] | directo |
| siigo_empresa | Empresa | [1:4] | directo |
| siigo_sync_hash | Hash | SHA256 | `fmt.Sprintf("%x", hash[:8])` |

---

#### PRODUCTOS — Prioridad ALTA

```
Archivo ISAM:    Z06CP
Parser:          parsers.ParseProductos(dataPath) → []Producto
Detector:        sync.detectProductosChanges() — clave: Comprobante-Secuencia
Conversion:      Producto.ToFinearomProduct() → map[string]interface{}
API Client:      api.Client.SyncProduct(data)
Endpoint HTTP:   POST /siigo/products
Clave de cruce:  Codigo comprobante-secuencia
```

**Campos que se envian a Finearom:**
```json
{
  "code":            "5-1",
  "product_name":    "CAMISA POLO RALPH LAUREN HOMBRE T-M",
  "grupo":           "999",
  "cuenta_contable": "0000000001000",
  "siigo_sync_hash": "a1b2c3d4e5f6g7h8"
}
```

**Campos ISAM usados:**
| Campo Finearom | Campo Producto | Offset EXTFH | Transformacion |
|----------------|---------------|-------------|----------------|
| code | Comprobante + Secuencia | [0:5] + [5:10] | `TrimLeft("0") + "-" + TrimLeft("0")` |
| product_name | Nombre | [46:86] | directo |
| grupo | Grupo | [11:14] | `TrimSpace()` |
| cuenta_contable | CuentaContable | [14:27] | `TrimSpace()` |

---

#### MOVIMIENTOS — Prioridad ALTA

```
Archivo ISAM:    Z49 (principal) + Z49YYYY (por ano)
Parser:          parsers.ParseMovimientos(dataPath) → []Movimiento
                 parsers.ParseMovimientosAnio(dataPath, "2014") → []Movimiento
Detector:        sync.detectMovimientosChanges() — clave: TipoComp-NumDoc-NitTercero
Conversion:      Movimiento.ToFinearomRecaudo() → map[string]interface{}
API Client:      api.Client.SyncMovement(data)
Endpoint HTTP:   POST /siigo/movements
Clave de cruce:  Tipo comprobante + numero documento
```

**Campos que se envian a Finearom:**
```json
{
  "nit":             "900019401",
  "numero_factura":  "6976",
  "fecha_recaudo":   "2013-01-31",
  "valor_cancelado": "",
  "descripcion":     "ABONO A FACT. CTA 01.",
  "siigo_sync_hash": "a1b2c3d4e5f6g7h8"
}
```

**Campos ISAM usados (EXTFH):**
| Campo Finearom | Campo Movimiento | Offset EXTFH | Transformacion |
|----------------|-----------------|-------------|----------------|
| nit | NitTercero | [15:50] o [4:15] | depende del tipo de registro |
| numero_factura | NumeroDoc | [4:15] | `TrimLeft("0")` |
| fecha_recaudo | Fecha | (no disponible en EXTFH) | viene de Z09 |
| descripcion | Descripcion | [72+] variable | `findDescripcion()` |

**Nota**: Z49 via EXTFH NO tiene fechas ni montos como texto. Para datos financieros completos, cruzar con Z09 (cartera) por NIT.

**Filtros utiles:**
- `Movimiento.IsReciboCaja()` → true si TipoComprobante == "RC" (recaudos)
- `Movimiento.IsFacturaVenta()` → true si TipoComprobante == "FV" (facturas)

---

#### CARTERA — Prioridad MEDIA

```
Archivo ISAM:    Z09YYYY (uno por ano: Z092013, Z092014, Z092015, Z092016)
Parser:          parsers.ParseCartera(dataPath, "2014") → []Cartera
                 parsers.ParseCarteraByTipo(dataPath, "2014", 'F') → []Cartera (solo facturas)
Detector:        sync.detectCarteraChanges() — clave: TipoRegistro-Empresa-Secuencia
Conversion:      Cartera.ToFinearomCartera() → map[string]interface{}
API Client:      api.Client.SyncCartera(data)
Endpoint HTTP:   POST /siigo/cartera
Clave de cruce:  NIT del tercero
```

**Campos que se envian a Finearom:**
```json
{
  "nit":              "900019401",
  "cuenta_contable":  "0004135050100",
  "fecha":            "2013-01-31",
  "tipo_movimiento":  "C",
  "descripcion":      "PORTATIL TOSHIBA DYNABOOK NX",
  "tipo_registro":    "F",
  "siigo_sync_hash":  "a1b2c3d4e5f6g7h8"
}
```

**Campos ISAM usados:**
| Campo Finearom | Campo Cartera | Offset EXTFH | Transformacion |
|----------------|--------------|-------------|----------------|
| nit | NitTercero | [16:29] | `TrimLeft("0")` |
| cuenta_contable | CuentaContable | [29:42] | directo |
| fecha | Fecha | [42:50] | `YYYY-MM-DD` formato con guiones |
| tipo_movimiento | TipoMov | [143] | D o C |
| descripcion | Descripcion | [93:133] | `TrimSpace()` |
| tipo_registro | TipoRegistro | [0] | F/G/L/P/R |

---

#### MAESTROS — Solo lectura (no se sincroniza a Finearom)

```
Archivo ISAM:    Z06
Parser:          parsers.ParseMaestros(dataPath) → []Maestro
                 parsers.ParseMaestrosPorTipo(dataPath, 'V') → []Maestro (solo vendedores)
Uso:             Datos de referencia para la app de escritorio (siigo-app)
                 No se envian a Finearom directamente
```

### 17.3 El detector de cambios (sync/detector.go)

El detector funciona asi:

1. **Revisa timestamp** del archivo ISAM con `isam.GetModTime(path)`
2. Si NO cambio desde la ultima ejecucion → skip (barato)
3. Si CAMBIO → parsea todo el archivo con el parser correspondiente
4. Genera un mapa `{clave: hash}` de todos los registros actuales
5. Compara con el mapa anterior (guardado en estado local)
6. Produce una lista de `[]Change` con tipo: `new`, `updated`, `deleted`

**Claves unicas por archivo:**
| Archivo | Clave del registro | Ejemplo |
|---------|-------------------|---------|
| Z17 | `TipoClave-Empresa-Codigo` | `G-001-00000000002002` |
| Z06CP | `Comprobante-Secuencia` | `5-1` |
| Z49 | `TipoComprobante-NumeroDoc-NitTercero` | `RC001-6976-` |
| Z09YYYY | `TipoRegistro-Empresa-Secuencia` | `F-001-00001` |

### 17.4 El cliente API (api/client.go)

```go
client := api.NewClient("https://finearom.com/api", "user@email.com", "password")

// 1. Login (obtiene Bearer token via Sanctum)
client.Login()

// 2. Sincronizar un cliente
data := tercero.ToFinearomClient()
client.SyncClient(data)    // POST /siigo/clients

// 3. Sincronizar un producto
data := producto.ToFinearomProduct()
client.SyncProduct(data)   // POST /siigo/products

// 4. Sincronizar un movimiento
data := movimiento.ToFinearomRecaudo()
client.SyncMovement(data)  // POST /siigo/movements

// 5. Sincronizar cartera
data := cartera.ToFinearomCartera()
client.SyncCartera(data)   // POST /siigo/cartera
```

### 17.5 Configuracion del sync service (config/config.go)

```json
{
  "siigo": {
    "data_path": "C:\\SIIWI02\\"
  },
  "finearom": {
    "base_url": "https://finearom.com/api",
    "email": "siigo-sync@empresa.com",
    "password": "..."
  },
  "sync": {
    "interval_seconds": 300,
    "files": ["Z17", "Z06CP", "Z49", "Z092024"],
    "state_path": "./sync_state.json"
  }
}
```

**`files`** define que archivos ISAM se monitorean. El detector se ejecuta cada `interval_seconds`.

### 17.6 Resumen: que tocar para agregar un nuevo dato a sincronizar

| Paso | Archivo | Que hacer |
|------|---------|-----------|
| 1 | `siigo-common/parsers/nuevo.go` | Crear struct + parser con offsets EXTFH/binary |
| 2 | `siigo-sync/sync/detector.go` | Agregar case en `DetectChanges()` switch + `detectNuevoChanges()` |
| 3 | `siigo-sync/api/client.go` | Agregar metodo `SyncNuevo(data)` con endpoint HTTP |
| 4 | `siigo-sync/cmd/peek/main.go` | Agregar seccion de validacion |
| 5 | `siigo-sync/main.go` | Agregar al loop de polling si es archivo nuevo |
| 6 | Config JSON | Agregar nombre del archivo a `sync.files[]` |
| 7 | Finearom Laravel | Crear endpoint `POST /siigo/nuevo` + migration + model |

### 17.7 Archivos ya conectados vs pendientes

| Dato | Archivo | Parser | Detector | API Client | Endpoint | Estado |
|------|---------|--------|----------|-----------|----------|--------|
| Clientes | Z17 | `ParseTercerosClientes()` | `detectTercerosChanges()` | `SyncClient()` | POST /siigo/clients | Listo en Go |
| Productos | Z06CP | `ParseProductos()` | `detectProductosChanges()` | `SyncProduct()` | POST /siigo/products | Listo en Go |
| Movimientos | Z49 | `ParseMovimientos()` | `detectMovimientosChanges()` | `SyncMovement()` | POST /siigo/movements | Listo en Go |
| Cartera | Z09YYYY | `ParseCartera()` | `detectCarteraChanges()` | `SyncCartera()` | POST /siigo/cartera | Listo en Go |
| Maestros | Z06 | `ParseMaestros()` | — | — | — | Solo lectura |
| Mov. contables | Z03YYYY | — | — | — | — | Pendiente |
| Plan cuentas | C03 | — | — | — | — | Pendiente |
| Empresa | Z001 | — | — | — | — | Pendiente |

**"Listo en Go"** = el codigo existe en siigo-sync pero los endpoints en Finearom (Laravel) aun no estan creados.
