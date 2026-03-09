# 10 - Lector ISAM V2: Especificacion Tecnica Completa

## Indice

1. [Contexto y Motivacion](#1-contexto-y-motivacion)
2. [Arquitectura de Lectura ISAM (3 capas)](#2-arquitectura-de-lectura-isam-3-capas)
3. [V1: Lector Heuristico (reader.go)](#3-v1-lector-heuristico-readergo)
4. [Investigacion del Formato Micro Focus IDXFORMAT 8](#4-investigacion-del-formato-micro-focus-idxformat-8)
5. [Inspeccion de Headers Reales (19 archivos Siigo)](#5-inspeccion-de-headers-reales-19-archivos-siigo)
6. [V2: Lector Basado en Especificacion (reader_v2.go)](#6-v2-lector-basado-en-especificacion-reader_v2go)
7. [Comparacion V1 vs V2 vs EXTFH (DLL)](#7-comparacion-v1-vs-v2-vs-extfh-dll)
8. [Ingenieria Inversa de cblrtsm.dll](#8-ingenieria-inversa-de-cblrtsmdll)
9. [Configuracion EXTFH.CFG del Runtime](#9-configuracion-extfhcfg-del-runtime)
10. [Integracion en Produccion](#10-integracion-en-produccion)
11. [Referencia Tecnica Completa](#11-referencia-tecnica-completa)
12. [Herramientas de Diagnostico](#12-herramientas-de-diagnostico)
13. [Lecciones Aprendidas](#13-lecciones-aprendidas)

---

## 1. Contexto y Motivacion

### El problema

Siigo Pyme almacena toda su contabilidad en archivos ISAM propietarios de Micro Focus (COBOL runtime). No existe documentacion publica del formato on-disk. Para leer estos archivos tenemos tres opciones:

1. **EXTFH (DLL)**: Llamar directamente a `cblrtsm.dll` via la interfaz EXTFH (External File Handler). Requiere la DLL instalada (solo Windows, solo con licencia Micro Focus).
2. **Binary Reader V1**: Lector heuristico que escanea los archivos buscando patrones que "parezcan" registros. No requiere DLL pero tiene falsos positivos.
3. **Binary Reader V2** (nuevo): Lector basado en la especificacion del formato Micro Focus, reimplementado desde cero usando documentacion abierta y ingenieria inversa.

### Por que se creo V2

V1 funcionaba "bien" para la mayoria de archivos, pero:
- Confundia paginas de indice con datos reales (hasta +234 registros falsos en ZDANE)
- No podia leer archivos con records >= 4096 bytes (Z06: 0 registros encontrados vs 587 reales)
- Usaba un "readability check" heuristico que filtraba registros validos con datos binarios
- No entendia la estructura interna del archivo (tipos de registro, alineacion, paginas de indice)

V2 fue creado para tener un fallback binario **identico** a la DLL oficial.

---

## 2. Arquitectura de Lectura ISAM (3 capas)

```
ReadIsamFile(path)  /  ReadIsamFileWithMeta(path)
        |
        v
   ExtfhAvailable()?
        |
   +----+----+
   |         |
  YES        NO
   |         |
   v         v
 EXTFH     V2 Reader      <-- Antes era V1, ahora es V2
 (DLL)     (spec-based)
   |         |
   v         v
 [][]byte   [][]byte      <-- Mismos datos, mismo formato
```

### Archivo: `siigo-common/isam/extfh.go`

La funcion `ReadIsamFile()` decide que lector usar:

```go
func ReadIsamFile(path string) ([][]byte, int, error) {
    if ExtfhAvailable() {
        // Opcion 1: DLL de Micro Focus (gold standard)
        f, err := OpenIsamFile(path)
        if err != nil { return nil, 0, err }
        defer f.Close()
        records, err := f.ReadAll()
        return records, f.RecSize, err
    }

    // Opcion 2: Lector binario V2 (spec-based) — ANTES era V1
    records, recSize, err := ReadFileV2All(path)
    if err != nil { return nil, 0, err }
    return records, recSize, nil
}
```

### Prioridad de lectores

| Prioridad | Lector | Archivo | Requiere | Precision |
|-----------|--------|---------|----------|-----------|
| 1 (preferido) | EXTFH | extfh.go | cblrtsm.dll + licencia | 100% (referencia) |
| 2 (fallback) | V2 | reader_v2.go | Nada (Go puro) | 100% (verificado) |
| 3 (deprecado) | V1 | reader.go | Nada (Go puro) | 53% (10/19 archivos) |

---

## 3. V1: Lector Heuristico (reader.go)

### Como funciona V1

1. **Lee header** — Extrae magic (offset 0x00) y record size (offset 0x38)
2. **Empieza en offset 0x800** — Asume que header + primer indice ocupan 2048 bytes
3. **Escanea byte por byte** buscando marcadores:
   ```
   byte[pos]   = statusNibble | recSizeHi    (upper 4 bits = status, lower 4 = high byte of recSize)
   byte[pos+1] = recSizeLo                    (low byte of recSize)
   ```
4. **Readability check** — Lee 30 bytes despues del marcador; si hay 15+ caracteres imprimibles (0x20-0xFE), lo acepta como registro
5. **Extrae datos** — Copia `recSize` bytes empezando 2 bytes despues del marcador

### Codigo clave de V1

```go
// Scan for records starting at offset 0x800
for pos := 0x800; pos < len(data)-recSize-2; pos++ {
    // Record marker: [statusNibble|recHi][recLo]
    if data[pos+1] != recLo || (data[pos]&0x0F) != recHi {
        continue
    }

    // Validate status nibble
    statusNibble := data[pos] & 0xF0
    if !validStatusNibbles[statusNibble] {
        continue
    }

    // Readability check: 15+ printable chars in first 30 bytes
    readableCount := 0
    for i := pos + 2; i < pos + 32; i++ {
        if data[i] >= 0x20 && data[i] < 0xFF {
            readableCount++
        }
    }
    if readableCount > 15 {
        // Accept as record
    }
}
```

### Debilidades de V1

| Problema | Descripcion | Archivos afectados |
|----------|-------------|-------------------|
| Falsos positivos | Paginas de indice pasan el readability check | Z07, Z07T, ZDANE, ZICA, Z03, Z04, Z09, Z08A |
| Sin soporte 4-byte | No maneja records >= 4096 bytes | Z06 (4096-byte records) |
| Sin alineacion | No respeta padding a 8 bytes | Todos (IDXFORMAT 8) |
| Sin tipos de registro | No distingue NULL, DELETED, HEADER, SYSTEM | Todos |
| Offset fijo 0x800 | Asume header de 2048 bytes | Funciona por suerte |

### Errores especificos de V1 por archivo

| Archivo | EXTFH | V1 | Falsos (+) / Faltantes (-) | Causa |
|---------|-------|-----|---------------------------|-------|
| Z06 | 587 | 0 | -587 | Magic 0x30000x03FC → V1 no reconoce 4-byte markers |
| ZDANE | 1119 | 1353 | +234 | Paginas de indice del B-tree parecen texto |
| Z07 | 64 | 106 | +42 | Index entries tienen formato similar a datos |
| Z07T | 116 | 163 | +47 | Mismo problema que Z07 |
| ZICA | 431 | 523 | +92 | Index entries con codigos legibles |
| Z03 | 2836 | 2842 | +6 | Algunos SYSTEM records pasan el filtro |
| Z04 | 122 | 126 | +4 | Index headers con texto |
| Z09 | 86 | 88 | +2 | Headers con datos parciales |
| Z08A | 87 | 89 | +2 | Similar a Z09 |

---

## 4. Investigacion del Formato Micro Focus IDXFORMAT 8

### Fuentes consultadas

Se investigaron 4 fuentes principales para entender el formato on-disk:

#### 4.1 SimoTime (simotime.com)
- Documentacion general de ISAM/EXTFH
- Referencia de opcodes EXTFH y FCD3 (File Control Description)
- Formatos numericos COBOL (packed decimal, display, comp)

#### 4.2 DataConversion-Framework (GitHub)
- Scripts de conversion entre formatos de archivos COBOL
- Deteccion de magic bytes y headers
- Referencia de IDXFORMAT values (0, 1, 2, 3, 4, 8)

#### 4.3 GnuCOBOL fileio.c (SourceForge)
- Implementacion open-source del file handler COBOL
- Codigo C que parsea headers de archivos Micro Focus
- Deteccion de magic bytes, organizacion, compresion

#### 4.4 mfcobol-export (GitHub) — **Fuente mas valiosa**
- Script Python que exporta datos de archivos Micro Focus ISAM
- **Documenta completamente el formato de record headers**:
  - 2-byte markers: top 4 bits = tipo, bottom 12 bits = longitud
  - 4-byte markers: top 5 bits = tipo, bottom 27 bits = longitud
  - Record types: NULL(0), SYSTEM(1), DELETED(2), HEADER(3), NORMAL(4), REDUCED(5), POINTER(6), REFDATA(7), REDREF(8)
- **Documenta alineacion por IDXFORMAT**:
  - IDXFORMAT 8: alineacion a 8 bytes
  - IDXFORMAT 3/4: alineacion a 4 bytes
  - Otros: sin alineacion

### Layout del Header de 128 bytes

Mapa completo de los 128 bytes del header, verificado con hex dumps de 19 archivos reales de Siigo:

```
Offset  Bytes  Campo                 Endian    Descripcion
------  -----  --------------------  --------  ----------------------------------
0x00    4      Magic                 Big       Firma del formato
0x04    2      DB Sequence           Big       Numero de secuencia de la DB
0x06    2      Integrity Flag        Big       0 = OK, !=0 = posible corrupcion
0x08    14     Creation Date         ASCII     YYMMDDHHMMSSCC
0x16    14     Modified Date         ASCII     YYMMDDHHMMSSCC
0x24    2      Reserved              -         Esperado: 0x00 0x3E
0x27    1      Organization          -         1=Seq, 2=Indexed, 3=Relative
0x29    1      Data Compression      -         0=None, 1=CBLDC001
0x2B    1      IDXFORMAT             -         0,1,2,3,4 o 8
0x30    1      Recording Mode        -         0=Fixed, 1=Variable
0x36    4      Max Record Length     Big       Tamanio maximo de registro
0x3A    4      Min Record Length     Big       Tamanio minimo de registro
0x6C    4      Handler Version      Big       Version del file handler (indexed)
0x78    8      Logical End          Big       Offset logico de fin de datos
```

### Magic Bytes (offset 0x00-0x03)

Cuatro variantes identificadas en archivos de Siigo:

| Magic Bytes | Nombre | Significado |
|-------------|--------|-------------|
| `30 7E 00 00` | SHORT_RECORDS | Formato estandar, markers de 2 bytes |
| `30 00 00 7C` | LONG_RECORDS | Records >= 4096 bytes, markers de 4 bytes |
| `33 FE xx xx` | MF_INDEXED | Formato indexado Micro Focus (mas comun en Siigo) |
| `30 xx xx xx` | MF_VARIANT | Variante (ej: Z06 usa `30 00 03 FC`) |

### Record Headers (Marcadores de Registro)

#### Formato de 2 bytes (records < 4096 bytes)

```
Bits:  [TTTT LLLL LLLL LLLL]
        ^^^^                   = Tipo de registro (4 bits, 0x0-0xF)
             ^^^^^ ^^^^^ ^^^^  = Longitud de datos (12 bits, 0-4095)
```

Ejemplo: Marker `0x45A0` → Tipo = 0x4 (NORMAL), Longitud = 0x5A0 = 1440 bytes

#### Formato de 4 bytes (records >= 4096 bytes)

```
Bits:  [TTTTT LLL LLLLLLLL LLLLLLLL LLLLLLLL]
        ^^^^^                                  = Tipo (5 bits)
              ^^^ ^^^^^^^^ ^^^^^^^^ ^^^^^^^^   = Longitud (27 bits)
```

Ejemplo: Marker `0x20001000` → Tipo = 0x4 (NORMAL >> ajustado), Longitud = 0x1000 = 4096 bytes

### Tipos de Registro (Record Types)

| Codigo | Nombre | Descripcion | Se extrae como dato? |
|--------|--------|-------------|---------------------|
| 0x0 | NULL | Padding/relleno sin datos | No |
| 0x1 | SYSTEM | Espacio libre (variable indexed) | No |
| 0x2 | DELETED | Registro eliminado, slot disponible para reuso | No |
| 0x3 | HEADER | Header de archivo o de pagina de indice | No |
| 0x4 | NORMAL | **Registro de datos normal** | **Si** |
| 0x5 | REDUCED | **Registro reducido (indexed)** | **Si** |
| 0x6 | POINTER | Puntero de indice a bloque de datos | No |
| 0x7 | REFDATA | **Registro de datos referenciado por puntero** | **Si** |
| 0x8 | REDREF | **Registro reducido referenciado por puntero** | **Si** |

Solo los tipos 4, 5, 7 y 8 contienen datos de usuario.

### Alineacion (Padding)

Despues de cada registro (header + datos), se agrega padding para alinear al siguiente multiplo:

| IDXFORMAT | Alineacion | Formula |
|-----------|-----------|---------|
| 8 | 8 bytes | `slack = (8 - (consumed % 8)) % 8` |
| 3, 4 | 4 bytes | `slack = (4 - (consumed % 4)) % 4` |
| 0, 1, 2 | 1 byte | Sin padding |

Ejemplo para un registro NORMAL de 1438 bytes con IDXFORMAT=8:
```
consumed = 2 (header) + 1438 (datos) = 1440
slack    = (8 - (1440 % 8)) % 8 = (8 - 0) % 8 = 0   → ya alineado
total    = 1440 bytes hasta el siguiente registro
```

Ejemplo para un registro de 518 bytes:
```
consumed = 2 + 518 = 520
slack    = (8 - (520 % 8)) % 8 = (8 - 0) % 8 = 0
total    = 520
```

Ejemplo para un registro de 256 bytes:
```
consumed = 2 + 256 = 258
slack    = (8 - (258 % 2)) % 8 = (8 - 2) % 8 = 6
total    = 264 (258 + 6 padding)
```

---

## 5. Inspeccion de Headers Reales (19 archivos Siigo)

Se creo la herramienta `siigo-sync/cmd/inspect_header/main.go` para inspeccionar los 128 bytes del header de todos los archivos ISAM de Siigo. Resultados:

### Todos los archivos confirmados

| Archivo | Magic | Org | IDXFORMAT | MaxRecLen | MinRecLen | Long? |
|---------|-------|-----|-----------|-----------|-----------|-------|
| Z17 | 0x33FE | Indexed(2) | 8 | 1438 | 1438 | No |
| Z06 | 0x30000x03FC | Indexed(2) | 8 | 4096 | 4096 | **Si** |
| Z49 | 0x33FE | Indexed(2) | 8 | 2295 | 2295 | No |
| Z032016 | 0x33FE | Indexed(2) | 8 | 1152 | 1152 | No |
| Z042016 | 0x33FE | Indexed(2) | 8 | 3520 | 3520 | No |
| Z092016 | 0x33FE | Indexed(2) | 8 | 1152 | 1152 | No |
| Z112016 | 0x33FE | Indexed(2) | 8 | 518 | 518 | No |
| Z182016 | 0x33FE | Indexed(2) | 8 | 524 | 524 | No |
| Z252016 | 0x33FE | Indexed(2) | 8 | 512 | 512 | No |
| Z262016 | 0x33FE | Indexed(2) | 8 | 1544 | 1544 | No |
| Z272016 | 0x33FE | Indexed(2) | 8 | 2048 | 2048 | No |
| Z282016 | 0x33FE | Indexed(2) | 8 | 512 | 512 | No |
| Z052016 | 0x33FE | Indexed(2) | 8 | 1023 | 1023 | No |
| Z072016 | 0x33FE | Indexed(2) | 8 | 256 | 256 | No |
| Z07T | 0x33FE | Indexed(2) | 8 | 256 | 256 | No |
| Z082016A | 0x33FE | Indexed(2) | 8 | 1152 | 1152 | No |
| ZDANE | 0x33FE | Indexed(2) | 8 | 256 | 256 | No |
| ZICA | 0x33FE | Indexed(2) | 8 | 256 | 256 | No |
| ZPILA | 0x33FE | Indexed(2) | 8 | 230 | 230 | No |

### Observaciones clave

1. **Todos son IDXFORMAT=8** — single-file (datos + indices en un solo archivo)
2. **Todos son Indexed** (Organization = 2) — usan B-tree para indices
3. **Todos son Fixed-length** (MinRecLen = MaxRecLen) — no hay registros de longitud variable
4. **Solo Z06 usa 4-byte markers** — porque su record size (4096) alcanza el limite de 12 bits
5. **Compression = 0 en todos** — no usan compresion CBLDC001
6. **Integrity = 0 en todos** — ninguno reporta corrupcion
7. **MaxRecLen@0x36 coincide con RecSize@0x38** — V1 usaba offset 0x38, V2 usa 0x36 (ambos funcionan porque Siigo siempre tiene Fixed-length records)

---

## 6. V2: Lector Basado en Especificacion (reader_v2.go)

### Filosofia de diseno

V2 implementa el formato Micro Focus ISAM tal como esta especificado, sin heuristicas ni "readability checks". Confia en la estructura del archivo.

### Estructuras de datos

#### V2Header (metadata del archivo)

```go
type V2Header struct {
    Magic          uint32  // Bytes 0-3: firma del formato
    LongRecords    bool    // true si markers de 4 bytes (records >= 4096)
    DBSequence     uint16  // Bytes 4-5: secuencia de la DB
    IntegrityFlag  uint16  // Bytes 6-7: flag de integridad
    CreationDate   string  // Bytes 8-21: YYMMDDHHMMSSCC
    ModifiedDate   string  // Bytes 22-35: YYMMDDHHMMSSCC
    Organization   byte    // Byte 39: 1=Seq, 2=Indexed, 3=Relative
    Compression    byte    // Byte 41: 0=None, 1=CBLDC001
    IdxFormat      byte    // Byte 43: IDXFORMAT (0,1,2,3,4,8)
    RecordMode     byte    // Byte 48: 0=Fixed, 1=Variable
    MaxRecordLen   uint32  // Bytes 54-57: tamanio maximo de registro
    MinRecordLen   uint32  // Bytes 58-61: tamanio minimo
    HandlerVersion uint32  // Bytes 108-111: version del handler
    LogicalEnd     uint64  // Bytes 120-127: fin logico del archivo

    // Campos derivados
    HeaderSize     int     // Siempre 128
    Alignment      int     // 8 para IDXFORMAT 8, 4 para 3/4, 1 para otros
    RecHeaderSize  int     // 2 o 4 bytes segun LongRecords
}
```

#### V2Stats (estadisticas de lectura)

```go
type V2Stats struct {
    Header       *V2Header
    TotalRecords int            // Registros de datos extraidos
    DeletedCount int            // Registros eliminados (tipo 2)
    NullCount    int            // Bloques NULL/padding (tipo 0)
    SystemCount  int            // Registros del sistema (tipo 1)
    HeaderCount  int            // Headers de pagina (tipo 3)
    PointerCount int            // Punteros de indice (tipo 6)
    DataTypes    map[byte]int   // Conteo por tipo de registro
}
```

### Algoritmo de lectura V2

```
1. Abrir archivo (con retry en caso de Windows sharing violation)
2. Parsear header de 128 bytes:
   a. Detectar magic bytes → determinar si usa 2 o 4 byte markers
   b. Leer Organization, IDXFORMAT, MaxRecordLen
   c. Calcular Alignment segun IDXFORMAT
3. Posicionar cursor en offset 128 (despues del header)
4. Loop principal:
   a. Leer marker (2 o 4 bytes)
   b. Extraer tipo (top bits) y longitud (bottom bits)
   c. Si tipo=NULL y longitud=0 → avanzar por Alignment, continuar
   d. Si longitud invalida → avanzar por Alignment, continuar
   e. Clasificar registro por tipo:
      - NORMAL(4), REDUCED(5), REFDATA(7), REDREF(8) → extraer datos
      - DELETED(2), SYSTEM(1), HEADER(3), POINTER(6) → contar, no extraer
      - NULL(0) → padding, saltar
   f. Para registros de datos: copiar MaxRecordLen bytes
   g. Calcular padding: slack = (Alignment - (consumed % Alignment)) % Alignment
   h. Avanzar pos += headerSize + dataLen + slack
5. Retornar registros extraidos
```

### Codigo completo del algoritmo de parsing

```go
for pos < len(data) {
    if pos+hdr.RecHeaderSize > len(data) {
        break
    }

    var recType byte
    var dataLen int

    if hdr.RecHeaderSize == 2 {
        marker := binary.BigEndian.Uint16(data[pos : pos+2])
        recType = byte(marker >> 12)          // top 4 bits
        dataLen = int(marker & 0x0FFF)        // bottom 12 bits
    } else {
        marker := binary.BigEndian.Uint32(data[pos : pos+4])
        recType = byte(marker >> 27)          // top 5 bits
        dataLen = int(marker & 0x07FFFFFF)    // bottom 27 bits
    }

    // NULL padding
    if recType == RecTypeNull && dataLen == 0 {
        pos += hdr.Alignment
        continue
    }

    // Validate
    if dataLen < 0 || dataLen > len(data)-pos-hdr.RecHeaderSize {
        pos += hdr.Alignment
        continue
    }

    // Classify
    isData := recType == RecTypeNormal || recType == RecTypeReduced ||
        recType == RecTypeRefData || recType == RecTypeRedRef

    if isData && dataLen > 0 {
        rec := make([]byte, recSize)
        copy(rec, data[dataStart:dataStart+copyLen])
        info.Records = append(info.Records, Record{Data: rec, Offset: pos})
    }

    // Advance with alignment
    consumed := hdr.RecHeaderSize + dataLen
    if hdr.Alignment > 1 {
        slack := (hdr.Alignment - (consumed % hdr.Alignment)) % hdr.Alignment
        consumed += slack
    }
    pos += consumed
}
```

### Funciones publicas de V2

| Funcion | Entrada | Salida | Uso |
|---------|---------|--------|-----|
| `ReadFileV2(path)` | Path del archivo | `(*FileInfo, *V2Header, error)` | Lectura completa con metadata |
| `ReadFileV2All(path)` | Path del archivo | `([][]byte, int, error)` | Compatible con ReadIsamFile |
| `ReadFileV2WithStats(path)` | Path del archivo | `([][]byte, *V2Stats, error)` | Diagnostico con estadisticas |
| `CompareV1V2(path)` | Path del archivo | (logs) | Comparacion diagnostica V1 vs V2 |

### Manejo de 4-byte markers (Z06)

Z06 es el unico archivo de Siigo con records de 4096 bytes, lo que activa el formato de 4-byte markers:

```
Magic: 0x30 0x00 0x03 0xFC   (variante MF, no 0x33FE)
MaxRecordLen: 4096 bytes
Record header: 4 bytes
  - Top 5 bits: tipo de registro
  - Bottom 27 bits: longitud de datos

Tipo de registros encontrados en Z06:
  - 0x2 (DELETED): 587 registros eliminados
  - 0x6 (POINTER): 87 punteros de indice
  - 0x8 (REDREF): 587 registros de datos ← datos reales

V1 resultado: 0 registros (no entiende 4-byte markers)
V2 resultado: 587 registros = igual que EXTFH
```

---

## 7. Comparacion V1 vs V2 vs EXTFH (DLL)

### Metodologia de prueba

Se creo `siigo-sync/cmd/compare_readers/main.go` que ejecuta los 3 lectores sobre los 19 archivos ISAM y compara:
1. Conteo de registros
2. Contenido byte-a-byte del primer y ultimo registro
3. Match total de todos los registros

### Resultados completos (19 archivos, 9,403 registros totales)

```
====================================================================
  ISAM READER 3-WAY COMPARISON: EXTFH (gold) vs V1 vs V2
====================================================================

Z17          (1438B)    EXTFH:   73   V1:   73   V2:   73   V1=MATCH  V2=MATCH  Data: 100%
Z06          (4096B)    EXTFH:  587   V1:    0   V2:  587   V1=MISS   V2=MATCH  Data: 100%
Z49          (2295B)    EXTFH: 3234   V1: 3234   V2: 3234   V1=MATCH  V2=MATCH  Data: 100%
Z032016      (1152B)    EXTFH: 2836   V1: 2842   V2: 2836   V1=+6     V2=MATCH  Data: 100%
Z042016      (3520B)    EXTFH:  122   V1:  126   V2:  122   V1=+4     V2=MATCH  Data: 100%
Z092016      (1152B)    EXTFH:   86   V1:   88   V2:   86   V1=+2     V2=MATCH  Data: 100%
Z112016      (518B)     EXTFH:   38   V1:   38   V2:   38   V1=MATCH  V2=MATCH  Data: 100%
Z182016      (524B)     EXTFH:   12   V1:   12   V2:   12   V1=MATCH  V2=MATCH  Data: 100%
Z252016      (512B)     EXTFH:  169   V1:  169   V2:  169   V1=MATCH  V2=MATCH  Data: 100%
Z262016      (1544B)    EXTFH:   34   V1:   34   V2:   34   V1=MATCH  V2=MATCH  Data: 100%
Z272016      (2048B)    EXTFH:   18   V1:   18   V2:   18   V1=MATCH  V2=MATCH  Data: 100%
Z282016      (512B)     EXTFH:  116   V1:  116   V2:  116   V1=MATCH  V2=MATCH  Data: 100%
Z052016      (1023B)    EXTFH:   22   V1:   22   V2:   22   V1=MATCH  V2=MATCH  Data: 100%
Z072016      (256B)     EXTFH:   64   V1:  106   V2:   64   V1=+42    V2=MATCH  Data: 100%
Z07T         (256B)     EXTFH:  116   V1:  163   V2:  116   V1=+47    V2=MATCH  Data: 100%
Z082016A     (1152B)    EXTFH:   87   V1:   89   V2:   87   V1=+2     V2=MATCH  Data: 100%
ZDANE        (256B)     EXTFH: 1119   V1: 1353   V2: 1119   V1=+234   V2=MATCH  Data: 100%
ZICA         (256B)     EXTFH:  431   V1:  523   V2:  431   V1=+92    V2=MATCH  Data: 100%
ZPILA        (230B)     EXTFH:  239   V1:  239   V2:  239   V1=MATCH  V2=MATCH  Data: 100%

====================================================================
  SUMMARY
  V1 matches EXTFH:  10/19 files (53%)
  V2 matches EXTFH:  19/19 files (100%)
  Total records: EXTFH=9,403  V1=9,245  V2=9,403
  V2 data match: 9,403/9,403 records byte-identical to EXTFH (100%)
====================================================================
```

### Desglose de record types por archivo (V2)

| Archivo | Normal | Reduced | RefData | RedRef | Deleted | Null | System | Header | Pointer |
|---------|--------|---------|---------|--------|---------|------|--------|--------|---------|
| Z17 | 73 | 0 | 0 | 0 | 0 | 111 | 0 | 4 | 0 |
| Z06 | 0 | 0 | 0 | 587 | 587 | 698 | 0 | 0 | 87 |
| Z49 | 3234 | 0 | 0 | 0 | 0 | 111 | 0 | 71 | 0 |
| Z03 | 2836 | 0 | 0 | 0 | 0 | 111 | 2836 | 346 | 0 |
| Z04 | 122 | 0 | 0 | 0 | 0 | 102 | 122 | 43 | 0 |
| Z09 | 86 | 0 | 0 | 0 | 0 | 93 | 86 | 39 | 0 |
| Z11 | 38 | 0 | 0 | 0 | 0 | 93 | 38 | 18 | 0 |
| Z18 | 12 | 0 | 0 | 0 | 0 | 111 | 12 | 2 | 0 |
| Z25 | 169 | 0 | 0 | 0 | 0 | 111 | 0 | 36 | 0 |
| Z26 | 34 | 0 | 0 | 0 | 0 | 111 | 34 | 4 | 0 |
| Z27 | 18 | 0 | 0 | 0 | 0 | 102 | 18 | 8 | 0 |
| Z28 | 116 | 0 | 0 | 0 | 0 | 111 | 0 | 10 | 0 |
| Z05 | 22 | 0 | 0 | 0 | 0 | 102 | 22 | 5 | 0 |
| Z07 | 64 | 0 | 0 | 0 | 0 | 93 | 64 | 28 | 0 |
| Z07T | 116 | 0 | 0 | 0 | 0 | 102 | 116 | 35 | 0 |
| Z08A | 87 | 0 | 0 | 0 | 0 | 102 | 87 | 30 | 0 |
| ZDANE | 1119 | 0 | 0 | 0 | 0 | 111 | 1119 | 95 | 0 |
| ZICA | 431 | 0 | 0 | 0 | 0 | 111 | 431 | 39 | 0 |
| ZPILA | 239 | 0 | 0 | 0 | 0 | 111 | 0 | 10 | 0 |

### Observaciones del desglose

1. **Z06 es unico**: Usa RedRef (tipo 8) en vez de Normal (tipo 4), con punteros (tipo 6)
2. **SYSTEM records**: Muchos archivos tienen tantos SYSTEM como NORMAL — son entradas de indice B-tree que V1 confundia con datos
3. **NULL records**: ~100-111 por archivo — son bloques de padding entre paginas
4. **HEADER records**: Varian de 2 a 346 — son headers de paginas del B-tree
5. **Deleted records**: Solo Z06 tiene eliminados (587), los demas tienen 0

---

## 8. Ingenieria Inversa de cblrtsm.dll

### Localizacion de la DLL

```
Ruta: C:\Microfocus\bin64\cblrtsm.dll
Version: Micro Focus COBOL Server 9.0.00184
Formato: PE32+ (64-bit)
Tamanio: 1,195,504 bytes (1.1 MB)
Total exports: 1,852 funciones
```

### Funciones relevantes para File I/O

Se extrajeron todas las funciones exportadas y se clasificaron las relevantes:

#### Entry point principal
| Funcion | Descripcion |
|---------|-------------|
| `EXTFH` | Entry point para todas las operaciones de archivo ISAM |

#### Funciones de archivo de alto nivel
| Funcion | Descripcion |
|---------|-------------|
| `CBL_OPEN_FILE` | Abrir archivo |
| `CBL_READ_FILE` | Leer archivo |
| `CBL_WRITE_FILE` | Escribir archivo |
| `CBL_CLOSE_FILE` | Cerrar archivo |
| `CBL_DELETE_FILE` | Eliminar archivo |
| `CBL_FLUSH_FILE` | Flush a disco |
| `CBL_COPY_FILE` | Copiar archivo |
| `CBL_RENAME_FILE` | Renombrar archivo |
| `CBL_CHECK_FILE_EXIST` | Verificar existencia |
| `CBL_FILE_LENGTH` | Obtener tamanio |
| `CBL_FILE_ERROR` | Obtener ultimo error |

#### Funciones de metadata
| Funcion | Descripcion |
|---------|-------------|
| `CBL_GET_FILE_INFO` | Obtener metadata de archivo |
| `CBL_GET_FILE_SYSTEM_INFO` | Info del sistema de archivos |
| `CBL_CHECK_EXTERNAL_FILE_FCD3` | Validar estructura FCD3 |
| `mF_GETFILEINFO` | Metadata interna (API privada) |
| `mF_GETIXBLKSZ` | Tamanio de bloque de indice (API privada) |

#### Funciones de bloqueo
| Funcion | Descripcion |
|---------|-------------|
| `CBL_GET_RECORD_LOCK` | Obtener lock de registro |
| `CBL_FREE_RECORD_LOCK` | Liberar lock |
| `CBL_TEST_RECORD_LOCK` | Probar si esta bloqueado |
| `CBL_UNLOCK` | Desbloquear |

#### Funciones internas del File Handler (API privada)
| Funcion | Descripcion |
|---------|-------------|
| `CBL_FHINIT` | Inicializar file handler |
| `CBL_CFGREAD_EXTFH` | Leer configuracion EXTFH |
| `CBL_CFGREAD_DYNFH` | Leer configuracion de file handler dinamico |
| `CBL_DATA_COMPRESS` | Compresion de datos (CBLDC001) |
| `mF_cf_block_create` | Crear bloque de datos |
| `mF_cf_block_destroy` | Destruir bloque |
| `mF_cf_block_getsize` | Tamanio de bloque |
| `mF_cf_block_write_buffer` | Escribir buffer a bloque |
| `mF_cf_key_create` | Crear clave de indice |
| `mF_fh_set_fe_stat` | Establecer file status |
| `mF_fh_set_id_stat` | Establecer ID status |
| `mF_fh_set_lasterror` | Establecer ultimo error |
| `mF_acufh_init` | Inicializar ACUCOBOL file handler |

#### File handlers alternativos
| Funcion | Descripcion |
|---------|-------------|
| `mF_acufh_init` | ACUCOBOL file handler (formato Vision) |
| `mF_acufh_external_data` | Datos externos ACUFH |
| `GET_MFDBFH_FUNCS` | Funciones del database file handler |

### Strings encontradas en el binario

#### Nombres de programas/modulos del file handler
```
CBLDC          — Rutina de compresion
CHECKFIL       — Verificador de archivos
CHECKKY2       — Verificador de claves
XFHLABEL       — Etiquetas EXTFH
XFH_TLOG       — Log de transacciones
CBLUTIL        — Utilidades COBOL
OUTDDFH        — Output DD file handler
ESDSFH         — ESDS file handler
FSVIEWC        — File system viewer
FHRDRPWD       — Password del file handler reader
FHRDRLNGPWD    — Password largo
FHRSUB         — Subrutina del file handler
FSMGRLNG       — File system manager largo
FSMGR          — File system manager
```

#### Parametros de configuracion del runtime

```
isam_block_size            — Tamanio de bloque ISAM (configurable)
lock_mode                  — Modo de bloqueo de registros
noretry_on_decl            — No reintentar en declaracion
skip_on_lock               — Saltar si esta bloqueado
long_filenames             — Nombres de archivo largos
cobol_locking              — Bloqueo estilo COBOL
optimized_seeks            — Seeks optimizados
max_file_handles           — Maximo de handles abiertos
same_proc_excl_detection   — Deteccion de exclusion en mismo proceso
```

#### Mensajes de error relevantes
```
"File \"%s\" organization differs."  — Error de organizacion incompatible
"[7074]  file is corrupted"          — Archivo corrupto detectado
"EXCEPTION_DATATYPE_MISALIGNMENT"    — Error de alineacion de datos
```

### Patron de Magic Numbers en el binario

| Magic | Formato | Encontrados |
|-------|---------|-------------|
| 0x33FE (Big-Endian) | ISAM indexed | 12 ocurrencias |
| 0x33FE (Little-Endian) | Referencia invertida | 6 ocurrencias |
| 0x307E (Big-Endian) | Short records | 3 ocurrencias |
| 0x307E (Little-Endian) | Referencia invertida | 3 ocurrencias |
| `.idx` (string) | Extension de indice | 1 ocurrencia |

La unica referencia a `.idx` confirma que IDXFORMAT 8 usa archivos single-file (sin archivo de indice separado).

### Conclusion de la ingenieria inversa

La DLL no revela funcionalidad que V2 no maneje:
1. **No hay compresion** — Los archivos de Siigo no usan CBLDC001 (byte 41 = 0)
2. **No hay encriptacion** — Ningun archivo usa encriptacion
3. **No hay file handlers alternativos** — Solo usa el handler ISAM nativo
4. **Las funciones de metadata** (`CBL_GET_FILE_INFO`, `mF_GETFILEINFO`) dan informacion que ya extraemos del header de 128 bytes
5. **`mF_GETIXBLKSZ`** podria dar el block size del indice, pero no es necesario para leer datos

---

## 9. Configuracion EXTFH.CFG del Runtime

El archivo `C:\Siigo\EXTFH.CFG` configura el comportamiento del file handler:

```ini
[XFH-DEFAULT]
IGNORELOCK=ON          ; No usar bloqueo de registros
IDXFORMAT=8            ; Formato de indice: single-file, alineacion 8 bytes
FILEMAXSIZE=8          ; Tamanio maximo de archivo: 8 GB
INDEXCOUNT=32          ; Maximo de indices por archivo: 32
SEQDATBUF=8192         ; Buffer de lectura secuencial: 8 KB
FASTREAD=ON            ; Lectura rapida (menos validaciones)
KEYCHECK=OFF           ; No verificar integridad de claves
READSEMA=OFF           ; No usar semaforos para lectura
```

### Significado de cada parametro

| Parametro | Valor | Descripcion |
|-----------|-------|-------------|
| `IGNORELOCK` | ON | Ignora locks de registros — permite leer aunque Siigo tenga el archivo abierto |
| `IDXFORMAT` | 8 | Formato de archivo: todo en uno (datos + indices), alineacion 8 bytes, B-tree moderno |
| `FILEMAXSIZE` | 8 | Maximo 8 GB por archivo (suficiente para PYME) |
| `INDEXCOUNT` | 32 | Hasta 32 claves/indices por archivo (COBOL permite multiples) |
| `SEQDATBUF` | 8192 | Buffer de 8 KB para lectura secuencial (performance) |
| `FASTREAD` | ON | Modo lectura rapida — omite verificaciones de integridad al leer |
| `KEYCHECK` | OFF | No verifica que las claves sean consistentes al leer (performance) |
| `READSEMA` | OFF | No usa semaforos para coordinar lecturas (solo lectura, no escritura) |

### Implicaciones para V2

- `IGNORELOCK=ON` → V2 usa `openWithRetry()` que tambien maneja sharing violations
- `IDXFORMAT=8` → V2 usa `Alignment=8`, confirmado por el header de cada archivo
- `FASTREAD=ON` + `KEYCHECK=OFF` → V2 no necesita validar claves, solo extraer datos
- `FILEMAXSIZE=8` → V2 lee todo el archivo en memoria, viable para archivos de Siigo (maximo ~15 MB)

---

## 10. Integracion en Produccion

### Cambio realizado

En `siigo-common/isam/extfh.go`, las funciones `ReadIsamFile()` y `ReadIsamFileWithMeta()` fueron modificadas para usar V2 como fallback en lugar de V1:

#### Antes (V1 como fallback):
```go
func ReadIsamFile(path string) ([][]byte, int, error) {
    if ExtfhAvailable() { ... } // DLL

    // Fallback: binary reader (V1 heuristico)
    info, err := ReadFile(path)          // reader.go
    records := make([][]byte, len(info.Records))
    for i, r := range info.Records { records[i] = r.Data }
    return records, info.RecordSize, nil
}
```

#### Despues (V2 como fallback):
```go
func ReadIsamFile(path string) ([][]byte, int, error) {
    if ExtfhAvailable() { ... } // DLL

    // Fallback: spec-based binary reader (V2)
    records, recSize, err := ReadFileV2All(path)  // reader_v2.go
    return records, recSize, nil
}
```

### Estado de cada lector

| Lector | Archivo | Estado | Uso |
|--------|---------|--------|-----|
| EXTFH | extfh.go | Activo (primera opcion) | Produccion con DLL disponible |
| V2 | reader_v2.go | Activo (fallback) | Produccion sin DLL |
| V1 | reader.go | Disponible (no usado) | Solo 3 herramientas CLI de diagnostico |

### Usos restantes de V1

```
siigo-common/cmd/peek_files/main.go    — Herramienta de diagnostico
siigo-sync/cmd/process_v2/main.go      — Script de procesamiento
siigo-sync/cmd/discover/main.go        — Descubridor de archivos
```

Estas herramientas siguen usando `isam.ReadFile()` (V1) directamente. No afectan la produccion porque el pipeline principal (`ReadIsamFile` → parsers → SQLite) ya usa V2.

---

## 11. Referencia Tecnica Completa

### Archivos del modulo isam/

```
siigo-common/isam/
├── extfh.go      — Wrapper EXTFH (DLL), FCD3, opcodes, ReadIsamFile()
├── reader.go     — V1: lector heuristico (deprecado como fallback)
├── reader_v2.go  — V2: lector spec-based (fallback activo)
├── config.go     — Constantes (MaxLockRetries, LockRetryDelay)
└── types.go      — Tipos compartidos (Record, FileInfo, IsamHeader)
```

### Constantes de record types

```go
const (
    RecTypeNull     = 0x0  // Padding
    RecTypeSystem   = 0x1  // Free space
    RecTypeDeleted  = 0x2  // Eliminado
    RecTypeHeader   = 0x3  // Header de pagina
    RecTypeNormal   = 0x4  // Dato normal
    RecTypeReduced  = 0x5  // Dato reducido
    RecTypePointer  = 0x6  // Puntero de indice
    RecTypeRefData  = 0x7  // Dato referenciado
    RecTypeRedRef   = 0x8  // Reducido referenciado
)
```

### Formula de alineacion

```
consumed = RecHeaderSize + DataLen
slack    = (Alignment - (consumed % Alignment)) % Alignment
nextPos  = currentPos + consumed + slack
```

### Deteccion de long records

```go
switch {
case data[0]==0x30 && data[1]==0x7E && data[2]==0x00 && data[3]==0x00:
    LongRecords = false   // SHORT: 2-byte markers
case data[0]==0x30 && data[1]==0x00 && data[2]==0x00 && data[3]==0x7C:
    LongRecords = true    // LONG: 4-byte markers
case data[0]==0x33 && data[1]==0xFE:
    LongRecords = false   // MF Indexed: 2-byte markers
case (data[0] & 0xF0) == 0x30:
    LongRecords = (MaxRecordLen >= 4096)  // Variant: depends on record size
}
```

---

## 12. Herramientas de Diagnostico

### inspect_header (inspeccion de headers)

```bash
cd siigo-sync && go run ./cmd/inspect_header/
```

Lee los 128 bytes del header de cada archivo ISAM y muestra:
- Magic bytes con interpretacion
- Organizacion, IDXFORMAT, compresion
- Tamanios de registro (max/min)
- Fechas de creacion y modificacion
- Hex dump completo del header
- Primeros 64 bytes despues del header

### compare_readers (comparacion 3-way)

```bash
cd siigo-sync && go run ./cmd/compare_readers/
```

Ejecuta los 3 lectores (EXTFH, V1, V2) sobre los 19 archivos y compara:
- Conteo de registros de cada lector
- Match/mismatch contra EXTFH (gold standard)
- Comparacion byte-a-byte del primer y ultimo registro
- Porcentaje de registros identicos
- Desglose de record types (V2)

### deep_validate (validacion profunda)

```bash
cd siigo-sync && go run ./cmd/deep_validate/
```

Valida los 19 parsers contra datos reales:
- Campos clave no vacios
- Fechas en rango valido (1990-2030)
- Valores BCD razonables
- Muestra distribuida de cada parser

---

## 13. Lecciones Aprendidas

### 1. La heuristica es fragil

V1 buscaba "algo que parezca un registro" — funcionaba en el 53% de los archivos. El otro 47% tenia paginas de indice que pasaban el readability check porque contenian texto (nombres, codigos) como claves del B-tree.

**Leccion**: Cuando el formato esta documentado (aunque sea parcialmente), siempre es mejor implementar la especificacion que adivinar con heuristicas.

### 2. Los index pages son el enemigo de los heuristicos

Los archivos ISAM Micro Focus almacenan datos e indices en el mismo archivo. Las paginas de indice contienen copias de las claves (que son texto legible), lo que hace que V1 las confunda con registros de datos. V2 los distingue por el tipo de registro (HEADER=3, SYSTEM=1, POINTER=6) que es imposible de detectar sin entender el formato.

### 3. 4-byte markers son raros pero existen

De 19 archivos, solo Z06 usa 4-byte markers (records de 4096 bytes). V1 simplemente no podia leerlo. La leccion es que hay que manejar ambos formatos aunque el caso sea raro.

### 4. La alineacion importa

Sin respetar la alineacion de 8 bytes, V2 se desincronizaria despues del primer registro. Cada registro debe avanzar exactamente `headerSize + dataLen + padding` bytes para llegar al siguiente marcador.

### 5. La DLL no esconde secretos

La ingenieria inversa de `cblrtsm.dll` (1,852 exports, 1.1 MB) confirmo que no hay funcionalidad oculta que V2 necesite. Los archivos de Siigo no usan compresion, encriptacion, ni file handlers alternativos.

### 6. EXTFH.CFG revela el contrato

Los parametros `IGNORELOCK=ON`, `FASTREAD=ON`, `KEYCHECK=OFF` confirman que Siigo esta configurado para lectura rapida sin bloqueos — exactamente el caso de uso que V2 implementa.

### 7. Verificar siempre contra el gold standard

La comparacion 3-way (EXTFH vs V1 vs V2) fue crucial. Sin ella, no habriamos sabido que V2 era 100% correcto ni que V1 tenia 9 archivos con errores. **Siempre tener un gold standard para validar.**

---

## Apendice A: Archivos Creados/Modificados

| Archivo | Tipo | Descripcion |
|---------|------|-------------|
| `siigo-common/isam/reader_v2.go` | Nuevo | Lector spec-based (513 lineas) |
| `siigo-common/isam/extfh.go` | Modificado | Fallback cambiado de V1 a V2 |
| `siigo-sync/cmd/inspect_header/main.go` | Nuevo | Inspector de headers (191 lineas) |
| `siigo-sync/cmd/compare_readers/main.go` | Nuevo | Comparador 3-way (185 lineas) |
| `docs/10-ISAM-READER-V2.md` | Nuevo | Este documento |

## Apendice B: Fecha y Version

- **Fecha de creacion**: 2026-03-08
- **DLL analizada**: Micro Focus COBOL Server 9.0.00184 (cblrtsm.dll, 64-bit)
- **EXTFH.CFG**: IDXFORMAT=8, IGNORELOCK=ON, FASTREAD=ON
- **Archivos probados**: 19 archivos ISAM en `C:\DEMOS01\`
- **Registros totales verificados**: 9,403 (byte-identical entre EXTFH y V2)
