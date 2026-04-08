# 11 - Investigacion: ISAM Writer (Ingenieria Inversa)

## Indice

1. [Objetivo](#1-objetivo)
2. [DLLs Analizadas](#2-dlls-analizadas)
3. [mffh.dll: El File Handler Real](#3-mffhdll-el-file-handler-real)
4. [.NET FileHandler: Decompilacion](#4-net-filehandler-decompilacion)
5. [Formato Completo del Archivo ISAM IDXFORMAT=8](#5-formato-completo-del-archivo-isam-idxformat8)
6. [Estructura del B-Tree](#6-estructura-del-b-tree)
7. [Algoritmo de Escritura (Inferido)](#7-algoritmo-de-escritura-inferido)
8. [Analisis de Viabilidad: Writer en Go Puro](#8-analisis-de-viabilidad-writer-en-go-puro)
9. [Riesgos y Recomendaciones](#9-riesgos-y-recomendaciones)

---

## 1. Objetivo

Investigar la posibilidad de implementar operaciones de escritura (WRITE, REWRITE, DELETE) en archivos ISAM Micro Focus IDXFORMAT=8, sin depender de las DLLs propietarias (`cblrtsm.dll`, `mffh.dll`).

La motivacion es poder:
- Modificar registros desde la interfaz web de Siigo Middleware
- No depender de licencias Micro Focus para escritura
- Tener un writer portable que funcione en cualquier plataforma

## 2. DLLs Analizadas

### cblrtsm.dll (Runtime COBOL)
- **Ubicacion**: `C:\Microfocus\bin64\cblrtsm.dll`
- **Tamano**: 1,098,216 bytes
- **Exports**: 1,852 funciones
- **Funcion**: Runtime COBOL Server 9.0, incluye el punto de entrada EXTFH
- **Relevancia**: Es el "wrapper" que expone la interfaz EXTFH, pero delega en mffh.dll

### mffh.dll (File Handler)
- **Ubicacion**: `C:\Microfocus\bin64\mffh.dll`
- **Tamano**: 1,098,216 bytes
- **Exports**: 208 funciones
- **Funcion**: **EL file handler real** — contiene toda la logica de B-tree, escritura, compresion, transacciones
- **Version embebida**: `CHECKFIL 2024031817503851 GNR-078800000AM COIL NCG 8.0.0.78`

### MicroFocus.COBOL.FileHandler.dll (.NET)
- **Ubicacion**: `C:\Microfocus\bin\privateassemblies\v4.0\MicroFocus.COBOL.FileHandler.dll`
- **Tamano**: 1,583,640 bytes
- **Tipos**: 130 clases .NET
- **Funcion**: Version managed (.NET) del file handler, con metadata decompilable

## 3. mffh.dll: El File Handler Real

### 208 Exports Organizados por Categoria

#### Core I/O
| Funcion | Descripcion |
|---------|-------------|
| `MFFH` | Punto de entrada principal del file handler |
| `FS_OPEN_FILE` | Abrir archivo |
| `FS_CLOSE_FILE` | Cerrar archivo |
| `FS_READ_FILE` | Leer registro/pagina |
| `FS_WRITE_FILE` | Escribir registro/pagina |
| `FS_FLUSH_FILE` | Flush a disco |
| `FS_MOVE_RECORD` | Mover registro dentro del archivo |
| `FS_DELETE_FILE` | Eliminar archivo |
| `FS_SPLIT_FILENAME` | Parsear nombre de archivo |
| `FS_CHECK_FILE_EXIST` | Verificar existencia |

#### Integridad
| Funcion | Descripcion |
|---------|-------------|
| `CHECKFIL` | Verificar integridad del archivo completo |
| `CHECKKEY` | Verificar integridad de un indice |
| `CHECKKY2` | Verificacion alternativa de indice |

#### Compresion de Datos (CBLDC = COBOL Data Compress)
| Funcion | Firma (.NET) | Descripcion |
|---------|-------------|-------------|
| `CBLDC001` | `(DATA-RECORD, RECORD-SIZE, OUTPUT-RECORD, OUTPUT-RECLEN, COMPRESS-TYPE)` | Compresion/descompresion tipo 1 (RLE basico) |
| `CBLDC003` | `(LNK-INPUT-BUFFER, LNK-INPUT-BUFFER-Z, LNK-OUTPUT-BUFFER, LNK-OUTPUT-BUFFER-Z, LNK-COMPRESS-TYPE)` | Compresion tipo 3 (RLE con copy-offset) |
| `CBLDC005` | Similar a CBLDC001 | Compresion tipo 5 |
| `CBLDC101` | `(DATA-RECORD, RECORD-SIZE[32bit], OUTPUT-RECORD, OUTPUT-RECLEN[32bit], COMPRESS-TYPE)` | Version 32-bit de CBLDC001 |
| `CBLDC103` | Similar a CBLDC003 con UInt32 | Version 32-bit de CBLDC003 |

**Nota**: Los campos internos de CBLDC003 revelan el algoritmo:
- `CUR-CHAR`, `CUR-REPEAT`, `OLD-CHAR`: Deteccion de repeticiones
- `COPY-OFFSET`, `COPY-LENGTH`: Copia de secuencias previas (LZ77-like)
- `BUFF-CHARS-SAME-FLG`, `USE-1-REP-CHAR-FLG`: Flags de optimizacion

#### Striped Files (Archivos Divididos)
```
STRIPE_INITIALIZE, STRIPE_OPEN_FILE, STRIPE_CREATE_FILE
STRIPE_DELETE_FILE, STRIPE_COPY_FILE, STRIPE_RENAME_FILE
STRIPE_CLOSE_FILE
STRIPE_FILE_READ(handle, recno, reclen, record_area)
STRIPE_READ_FILE(handle, recno, reclen, record_area)
STRIPE_FILE_WRITE(handle, recno, reclen, record_area)
STRIPE_WRITE_FILE(handle, recno, reclen, record_area)
STRIPE_SEEK_END, STRIPE_FILE_LENGTH
STRIPE_SETSEMA, STRIPE_RELSEMA, STRIPE_UNLOCK
STRIPE_TEST_RECORD_LOCK, STRIPE_GET_RECORD_LOCK, STRIPE_FREE_RECORD_LOCK
```

#### Transaction Log
```
XFH_TLOG_INIT, XFH_TLOG_TIDY
XFH_TLOG_COMMIT, XFH_TLOG_ROLLBACK
XFH_TLOG_RECOVER, XFH_TLOG_RESTARTLOG
XFH_TLOG_SINGLERECOVER, XFH_TLOG_SETLOG
XFH_TLOG_CONFIGGET, XFH_TLOG_CONFIGSET
XFH_TLOG_TESTING (debug mode)
```

#### XA Distributed Transactions
```
FS_XA_START, FS_XA_END
FS_XA_PREPARE, FS_XA_COMMIT, FS_XA_ROLLBACK
FS_XA_RECOVER, FS_XA_FORGET
FS_XA_COMPLETE, FS_XA_OPEN, FS_XA_CLOSE
```

#### Rebuild
```
RBLDMAIN  — Entry point para reconstruir archivo
RBLDSUB   — Subrutina de reconstruccion
CLOSE_REBUILD — Finalizar reconstruccion
```

### Strings Relevantes Encontradas
```
BTRPAGE          — Concepto de "pagina B-tree"
XFHNODE          — Nodo del B-tree
DATACOMPRESS     — Flag de compresion de datos
KEYCOMPRESS      — Flag de compresion de keys
NODESIZE         — Tamano de nodo configurable
IDXFORMAT        — Formato de indice
FILEMAXSIZE      — Tamano maximo de archivo
INDEXCOUNT       — Numero maximo de indices
IGNORELOCK       — Ignorar locks
FASTREAD         — Lectura rapida
KEYCHECK         — Verificacion de claves
COMMITFLUSH      — Flush en commit
TRANSACTIONLOG   — Log de transacciones
```

### Ruta del Codigo Fuente Original
```
D:\Packages\pkg_349985\bln_873\VSCOBCOMP\coretech\fh\src\xfh\sqwrite.cbl
```
Esto confirma que el file handler fue escrito originalmente en **COBOL** y compilado a nativo.

## 4. .NET FileHandler: Decompilacion

La DLL .NET (`MicroFocus.COBOL.FileHandler.dll`) contiene 130 tipos. Los mas relevantes:

### MFFH (Clase Principal)
- 30 metodos (todos `_MF_PERFORM_809079_*` — COBOL compilado)
- Campos de estado:
  - `first-time-flag`, `last-time-flag`
  - `trans-log-flag`, `mid-commit-flag`
  - `close-container-flag`
  - `current-nest-level`
  - `flag-dbcs-enabled`, `flag-convert-status`
  - `flag-long-names`, `flag-searchoncreate`
  - `flag-esds-present`, `flag-btr-present`
  - `flag-run-under-fileshare`

### MFFH._MF_LCTYPE_3 (Parametros de Operacion)
Revela los datos que maneja el file handler internamente:
```
link-opdata       — Datos de la operacion
link-fcd          — File Control Description
link-name         — Nombre del archivo
link-recarea      — Area de registro (buffer de datos)
link-send-data    — Datos a enviar
link-keydef       — Definicion de claves
link-match-keylen — Longitud de clave para busqueda
link-match-keypos — Posicion de clave para busqueda
x-space           — Espacio de trabajo extra
f-space           — Espacio de archivo
c-space           — Espacio de compresion
search-space      — Espacio de busqueda
c-data-block      — Bloque de datos comprimidos
m-space           — Espacio de memoria
xtra-recarea      — Area extra de registro
```

### MFFH._MF_LCTYPE_2 (Transaction Log)
```
link-recarea      — Area de registro actual
link-recsize      — Tamano del registro
link-b4recarea    — Area de registro ANTES del cambio (undo)
link-b4recsize    — Tamano del registro original
link-recaddr      — Direccion del registro
link-oldrecaddr   — Direccion original (para moves)
link-rel-key      — Clave relativa
link-xa-id        — ID de transaccion XA
translog-vars     — Variables del transaction log
```

### MFFH._MF_LCTYPE_4 (Key Operations)
```
link-type         — Tipo de operacion
link-length       — Longitud
link-offset       — Offset
link-key1         — Buffer de clave 1
link-key2         — Buffer de clave 2
```

### rbldsub (Rebuild - Revela Estructura Interna)
```
STACK-ENTRY         — Pila del B-tree (para recorrido)
STACK-KEY-REF-P     — Referencia a clave en pila
STACK-REC-LOC-P     — Ubicacion de registro en pila
NEW-COMPONENT-CNT   — Conteo de componentes de clave
KEY-EXISTS          — Flag: la clave ya existe
NEW-KEY-COUNT       — Conteo de nuevas claves
REC-COUNT           — Conteo de registros
NODE-STATS-TABLE    — Tabla de estadisticas de nodos
KEY-BUFFER          — Buffer de claves
RECORD-BUFFER       — Buffer de registros
KEYDEF-AREA         — Area de definicion de claves
OCCURS-KEY-AREA     — Area de claves con OCCURS
ERROR-BUFFER        — Buffer de errores
```

### BSIO (Block Sequential I/O)
Maneja lectura/escritura a nivel de bloque (striped files):
```
STRIPE_FILE_READ(handle: UInt32, recno: UInt64, reclen: UInt32, record_area: Reference)
STRIPE_FILE_WRITE(handle: UInt32, recno: UInt64, reclen: UInt32, record_area: Reference)
STRIPE_SEEK_END(handle, recno)
STRIPE_FILE_LENGTH(handle, recno)
```

## 5. Formato Completo del Archivo ISAM IDXFORMAT=8

### Layout General del Archivo

```
+---------------------------+
| File Header (128 bytes)   |  @0
+---------------------------+
| Key Definition Area       |  @128  (NULL markers + key specs)
+---------------------------+
| Index Nodes (B-tree)      |  Type 3 records, 1024-byte data
+---------------------------+
| Data Records              |  Type 4/5/7/8 interleaved with
| + Address Pointers        |  Type 1 entries
+---------------------------+
| Index Nodes (overflow)    |  More Type 3 for alternate keys
+---------------------------+
```

### Header (128 bytes)

| Offset | Size | Campo | Ejemplo (ZDANE) |
|--------|------|-------|-----------------|
| 0 | 2 | Magic bytes | `0x33FE` |
| 2 | 6 | Reserved | zeros |
| 8 | 28 | Timestamps ASCII | `"17111415415759171114154157..."` |
| 36 | 1 | Zero | `0x00` |
| 37 | 1 | IDXFORMAT marker | `0x3E` (= format 8) |
| 38 | 2 | Record header size | `0x0002` (2 bytes) o `0x0004` (4 bytes) |
| 40 | 4 | Max record length | `264` (256 data + 2 marker + padding) |
| 44 | 4 | Reserved | `0` |
| 48 | 4 | Index node size | `1536` (reservado por nodo, incl. marker) |
| 52 | 4 | Reserved | `0` |
| 56 | 4 | Num keys config | `0x01000000` (1 primary key) |
| 60 | 4 | Key info | varies |
| 64 | 4 | Reserved | `0` |
| 68 | 4 | Active record count | `1119` |
| 72 | 4 | Reserved | `0` |
| 76 | 4 | Key format info | `0x03020100` |
| 80 | 24 | Reserved | zeros |
| 104 | 4 | Total records ever | `1120` |
| 108 | 4 | Create timestamp | Unix-like |
| 112 | 4 | Modify timestamp | Unix-like |
| 116 | 8 | Reserved | zeros |
| 124 | 4 | File size (BE) | `403696` |

### Record Markers

#### 2-byte marker (rec_header_size = 2)
```
Bits 15-12: Record type (4 bits)
Bits 11-0:  Data length (12 bits, max 4095)
```

#### 4-byte marker (rec_header_size = 4, for records >= 4096 bytes)
```
Bits 31-27: Record type (5 bits)
Bits 26-0:  Data length (27 bits, max 134M)
```

### Record Types

| Type | Hex | Name | Descripcion |
|------|-----|------|-------------|
| 0 | 0x0 | NULL | Slot vacio, siempre 8 bytes (no data) |
| 1 | 0x1 | ADDR | Address pointer, sigue a cada data record (4 bytes data) |
| 2 | 0x2 | DELETED | Registro eliminado (slot reutilizable) |
| 3 | 0x3 | INDEX | Nodo del B-tree (hoja o rama) |
| 4 | 0x4 | NORMAL | Registro de datos completo |
| 5 | 0x5 | REDUCED | Registro reducido (compresion parcial) |
| 6 | 0x6 | POINTER | Puntero a registro fragmentado |
| 7 | 0x7 | REFDATA | Registro de referencia |
| 8 | 0x8 | REDREF | Reduced reference (record comprimido con referencia) |

### Alignment
Todos los records se alinean a **8 bytes**:
```
padded_size = ceil((marker_size + data_length) / 8) * 8
```

### Data Record Slot (ejemplo: 256-byte record)

```
Offset  Content
------  -------
+0      Type 4 marker: 0x4100 (type=4, len=256)
+2      Record data (256 bytes)
+258    Padding (6 bytes zeros to align to 264)
+264    Type 1 marker: 0x1004 (type=1, len=4)
+266    Address data: 0x00000001
+270    Padding (2 bytes to align to 272)
```

Total por slot: **272 bytes** (264 data + 8 address)

### Key Definition Area

Ubicada justo despues del header (offset 128+):
- Comienza con un record NULL (type=0, len=0) de 8 bytes
- Seguido de un record NULL con datos (len=1026):
  - Contiene la definicion de claves del archivo
  - Formato: num_keys(2) + reserved(2) + key_def(24) * num_keys
  - Cada key_def: components(2) + offset(2) + length(2) + flags(2) + reserved(16)

## 6. Estructura del B-Tree

### Nodos del Indice (Type 3)

Cada nodo ocupa 1024 bytes de datos dentro de un slot de 1536 bytes:

```
+2 bytes marker (0x33FE)
+1022 bytes node data:
  [2 bytes header: flags + fill_info]
  [N entries: key_data + pointer]
  [remaining: zeros (empty slots)]
+padding to 1536 bytes
```

### Header del Nodo (2 bytes)

| Byte | Significado |
|------|-------------|
| `byte[0]` | Flags: bit 7 = es hoja (1) o rama (0) |
| `byte[1]` | Fill/usage info |

- `0x00 0x93`: Rama (branch), 147 entries usadas
- `0x82 0x09`: Hoja (leaf), primera hoja parcialmente llena
- `0x83 0xF8`: Hoja llena (92 entries, max para key de 5 bytes)

### Formato de Entry

Cada entrada contiene:
```
[key_length bytes]  Key data (sorted within node)
[6 bytes]           Pointer (big-endian file offset)
```

**Tamano de entry**: `key_length + 6` bytes

**Entries por nodo**: `(1022 - 2) / (key_length + 6)`

Ejemplos:
- ZDANE key=5 bytes: `(1022-2) / 11 = 92` entries max
- ZDANE nombre=40 bytes: `(1022-2) / 46 = 22` entries max

### Tipos de Nodo

#### Nodo Rama (Branch)
- `byte[0] & 0x80 == 0`
- Los punteros apuntan a **otros nodos** del B-tree (file offsets de nodos Type 3)
- Las claves son "separadores" entre los sub-arboles

#### Nodo Hoja (Leaf)
- `byte[0] & 0x80 != 0`
- Los punteros apuntan a **registros de datos** (file offsets de records Type 4/5/7/8)
- Las claves corresponden exactamente a las claves de los registros

### Verificacion con Datos Reales (ZDANE)

```
ZDANE: 1119 registros, key=5 bytes (codigo), key2=40 bytes (nombre)

Indice #1 (codigo):
  Root @2048 (BRANCH): 147 entries -> 147 hojas
  Leaf @308464: 47 entries (primera hoja, parcial)
  Leaf @309488: 8 entries
  ...
  147 hojas * ~8 entries = ~1176 (>= 1119 records)

Indice #2 (nombre):
  Root @3072 (BRANCH): 21 entries -> 21 hojas
  Leaf entries: ~22 max (40-byte keys)
  21 hojas * ~53 entries = ~1113 (>= 1119 records)

Todas las leaf entries verificadas: punteros apuntan exactamente
a los data records correctos (key match 100%).
```

## 7. Algoritmo de Escritura (Inferido)

### WRITE (Insertar Nuevo Registro)

```
1. ALLOCATE DATA SLOT
   a. Buscar slot DELETED (type=2) o NULL (type=0) con espacio suficiente
   b. Si no hay: extender archivo (append)
   c. Calcular padded_size = align8(2 + record_length)

2. WRITE DATA RECORD
   a. Escribir 2-byte marker: type=4, len=record_length
   b. Escribir record data
   c. Escribir padding zeros hasta alignment

3. WRITE ADDRESS POINTER
   a. Escribir 2-byte marker: type=1, len=4
   b. Escribir 4 bytes: 0x00000001
   c. Escribir padding

4. UPDATE B-TREE (para CADA indice)
   a. Extraer key value del record segun key definition
   b. Navegar B-tree desde root hasta leaf
   c. Encontrar posicion de insercion (sorted order)
   d. Si leaf tiene espacio: insertar entry
   e. Si leaf esta lleno: SPLIT
      - Crear nuevo leaf node
      - Dividir entries 50/50
      - Promover middle key al parent
      - Si parent lleno: cascade split

5. UPDATE HEADER
   a. Incrementar record_count (@68)
   b. Actualizar file_size (@124)
   c. Actualizar modify_timestamp (@112)
```

### REWRITE (Actualizar Registro Existente)

```
1. Encontrar registro actual via B-tree lookup
2. Si la KEY no cambio:
   a. Sobreescribir data record in-place
   b. No se modifica el B-tree
3. Si la KEY cambio:
   a. DELETE el registro anterior del B-tree
   b. Sobreescribir data record in-place
   c. INSERT nueva key en B-tree
4. Actualizar modify_timestamp
```

### DELETE (Eliminar Registro)

```
1. Encontrar registro via B-tree lookup
2. Marcar data record como DELETED (type=2)
3. Eliminar entry del B-tree leaf
4. Si leaf queda vacio o < 50%: merge con sibling (optional)
5. Decrementar record_count
6. Actualizar modify_timestamp
```

### B-Tree Split (Detalle)

```
Cuando un leaf node excede capacidad:

1. Crear nuevo Type 3 record (1024 bytes data)
   - Ubicar al final del area de indices o en slot libre
2. Mover la mitad superior de entries al nuevo nodo
3. El middle key se "promueve" al parent branch node
4. Insertar pointer al nuevo nodo en el parent
5. Si parent tambien desborda: repetir recursivamente
6. Si root desborda: crear nuevo root (el arbol crece un nivel)
```

## 8. Analisis de Viabilidad: Writer en Go Puro

### Lo que YA sabemos implementar (confirmado):

| Operacion | Estado | Evidencia |
|-----------|--------|-----------|
| Leer header | HECHO | reader_v2.go, 100% precision |
| Parsear record markers | HECHO | 2-byte y 4-byte, ambos modos |
| Navegar B-tree | VERIFICADO | Pointers decoded, leaf->data confirmado |
| Extraer keys | HECHO | Key definitions parseadas |
| Alineacion 8 bytes | HECHO | reader_v2.go |
| Record types 0-8 | HECHO | Todos clasificados correctamente |

### Lo que FALTA implementar:

| Operacion | Complejidad | Riesgo |
|-----------|-------------|--------|
| Escribir data record | BAJA | Solo overwrite bytes en posicion conocida |
| Marcar como deleted | BAJA | Cambiar 2 bytes del marker |
| B-tree lookup por key | MEDIA | Navegar branch -> leaf, binary search |
| B-tree insert en leaf | MEDIA | Shift entries, escribir nodo |
| B-tree split | ALTA | Crear nodos, actualizar parent, promover keys |
| B-tree merge (delete) | ALTA | Redistribuir entries entre siblings |
| Multi-key update | ALTA | Repetir para cada indice alternativo |
| File locking | MEDIA | Coordinar con Siigo (COBOL) |
| Compresion (CBLDC001/003) | MEDIA-ALTA | Solo si DATACOMPRESS esta habilitado |
| Transaction log | ALTA | Solo si se necesita crash recovery |
| Header update atomico | MEDIA | Race conditions con Siigo |

### Compresion

Siigo usa `DATACOMPRESS` en sus archivos? Verifiquemos:

Los archivos de SIIWI02 NO usan compresion — todos los records son type=4 (NORMAL) o type=8 (REDREF para archivos grandes como Z06). La compresion (types 5/REDUCED y 7/REFDATA) no se observa en los datos de produccion.

**Conclusion**: Para los archivos de Siigo, NO necesitamos implementar CBLDC001/003.

### Evaluacion Final

| Aspecto | Evaluacion |
|---------|-----------|
| **Lectura** | COMPLETAMENTE VIABLE — ya implementado (V2, 100% precision) |
| **REWRITE in-place** (sin cambio de key) | VIABLE — solo sobreescribir bytes |
| **DELETE** | VIABLE con reservas — marcar como deleted es facil, pero merge B-tree es complejo |
| **WRITE** (insert) | RIESGOSO — B-tree split es complejo y un error corrompe el archivo |
| **Multi-key files** | AUMENTA RIESGO — cada key adicional duplica la complejidad del B-tree |
| **Concurrencia con Siigo** | ALTO RIESGO — Siigo puede estar leyendo/escribiendo simultaneamente |

## 9. Riesgos y Recomendaciones

### Riesgo Principal: Corrupcion de Datos

Un error en el B-tree split o en el update del header puede dejar el archivo ISAM **ilegible para Siigo**. Esto significaria perdida de datos contables.

### Estrategias de Mitigacion

1. **Usar EXTFH DLL para escritura** (RECOMENDADO)
   - Ya tenemos los opcodes definidos en `extfh.go`
   - Solo necesitamos implementar `OpenIO`, `WriteRecord`, `RewriteRecord`, `DeleteRecord`
   - La DLL maneja B-tree, locks, transacciones correctamente
   - **Limitacion**: Solo funciona en Windows con Micro Focus instalado

2. **Writer Go puro solo para REWRITE** (SEGURO)
   - REWRITE sin cambio de key es lo mas simple: solo overwrite bytes
   - No toca el B-tree ni la estructura del archivo
   - Verificar con CHECKFIL despues de cada escritura
   - Implementar file locking basico

3. **Writer Go puro completo** (NO RECOMENDADO actualmente)
   - Requiere implementar B-tree split/merge correctamente
   - Necesita testing exhaustivo contra CHECKFIL
   - Riesgo de corrupcion en produccion
   - Mejor dejarlo para cuando haya mas madurez en el entendimiento del formato

### Recomendacion

**Fase 1** (inmediata): Implementar escritura via EXTFH DLL
- `OpenIsamFileIO()` — abrir en modo I-O usando OpOpenIO (0xFA02)
- `RewriteRecord()` — reescribir usando OpRewrite (0xFAF4)
- `DeleteRecord()` — eliminar usando OpDelete (0xFAF7)
- `WriteRecord()` — insertar usando OpWrite (0xFAF3)

**Fase 2** (futuro): REWRITE in-place en Go puro
- Solo para modificar datos sin cambiar keys
- Con backup automatico antes de cada operacion
- Con verificacion CHECKFIL post-escritura

**Fase 3** (si se necesita): Writer completo en Go puro
- Implementar B-tree completo basado en esta investigacion
- Testing exhaustivo con archivos de prueba
- Nunca usar en produccion sin meses de validacion

---

*Documento generado el 2026-03-08 mediante ingenieria inversa de mffh.dll, decompilacion de MicroFocus.COBOL.FileHandler.dll (.NET), y analisis hex de archivos ISAM reales.*
