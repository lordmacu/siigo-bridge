# EXTFH Wrapper - Detalles Tecnicos

## Que es EXTFH?

EXTFH (Extended File Handler) es la API de Micro Focus para acceder archivos ISAM desde programas externos. La funcion esta exportada por `cblrtsm.dll` con la firma:

```c
void EXTFH(unsigned char *opcode, FCD3 *fcd);
```

Nuestro wrapper en Go (`isam/extfh.go`) llama a esta funcion via `syscall.NewLazyDLL` + `syscall.NewProc`.

## Ubicacion del Codigo

- `siigo-sync/isam/extfh.go` - Wrapper EXTFH completo
- `siigo-sync/isam/reader.go` - Binary fallback + API unificada
- `siigo-app/isam/extfh.go` - Copia identica
- `siigo-app/isam/reader.go` - Copia identica

## FCD3 (File Control Description)

Estructura de 280 bytes que describe la operacion a realizar. Layout en 64-bit:

```
Offset  Bytes  Campo              Descripcion
--------------------------------------------------------------
0x00    1      fcd-status-1       Status byte 1 (9 = error)
0x01    1      fcd-status-2       Status byte 2 (065=file lock, 068=record lock)
0x04    2      fcd-organization   0=line-seq, 1=seq, 2=indexed, 3=relative
0x06    1      fcd-access-mode    0=seq, 4=random, 8=dynamic
0x07    1      fcd-open-mode      0=input, 1=output, 2=I-O, 3=extend
0x14    2      fcd-name-length    Longitud del nombre de archivo
0x1C    2      fcd-key-id         Indice de la clave (0=primaria)
0x32    2      fcd-key-length     Longitud de la clave activa
0x38    4      fcd-current-rec-len  Tamano del registro actual
0x3C    4      fcd-max-rec-len    Tamano maximo del registro
0x58    8      fcd-filename-ptr   Puntero al nombre de archivo (64-bit)
0x60    8      fcd-record-ptr     Puntero al buffer de registro (64-bit)
0x80    8      fcd-kdb-ptr        Puntero al Key Definition Block (64-bit)
```

Tamano total: 280 bytes (alineado para 64-bit).

## KDB (Key Definition Block)

Define las claves del archivo indexado. Soporta hasta 32 claves.

### Estructura del KDB

```
Offset  Bytes  Campo
--------------------------------------------------------------
HEADER (14 bytes):
0x00    2      kdb-total-length    Tamano total del KDB
0x02    4      filler
0x06    2      kdb-num-keys        Numero de claves
0x08    6      filler

KEY ENTRIES (16 bytes cada una, nkeys entradas):
+0x00   2      comp-count          Numero de componentes
+0x02   2      comp-offset         Offset en KDB a las definiciones de componentes
+0x04   1      key-flags           0x10=primary, 0x40=duplicates, 0x02=sparse
+0x05   1      comp-flags          Compression flags
+0x06   1      sparse-char         Sparse character
+0x07   9      reserved

COMPONENT ENTRIES (8 bytes cada una, despues de las key entries):
+0x00   2      rec-offset          Offset del componente dentro del registro
+0x02   2      comp-length         Longitud del componente en bytes
+0x04   2      comp-type           0=alphanumeric, otro=numerico/binario
+0x06   2      filler
```

### Extraccion automatica de claves

El wrapper extrae offset y length de cada componente de clave del KDB.
Esto permite construir un `IsamIndex` automaticamente sin hardcodear offsets:

```go
f, _ := isam.OpenIsamFile(`C:\DEMOS01\Z17`)
defer f.Close()

// f.Keys[0] = primary key con Components[].Offset y Components[].Length
// Extraer la clave de un registro:
keyBytes := f.Keys[0].ExtractKey(record)
keyString := f.Keys[0].ExtractKeyString(record)

// O usar ReadIsamFileAutoIndexed para indexar automaticamente por clave primaria:
idx, _ := isam.ReadIsamFileAutoIndexed(`C:\DEMOS01\Z17`, fallbackKeyFn)
rec := idx.Lookup("G0010000000000020")
```

## Opcodes Principales

### Apertura/Cierre
| Opcode | Valor | Descripcion |
|--------|-------|-------------|
| OPEN INPUT | 0xFA00 | Abrir solo lectura |
| OPEN I-O | 0xFA02 | Abrir lectura-escritura |
| CLOSE | 0xFA80 | Cerrar archivo |

### Lectura Secuencial
| Opcode | Valor | Descripcion |
|--------|-------|-------------|
| STEP FIRST | 0xFACC | Posicionar al primer registro |
| STEP NEXT | 0xFACA | Avanzar al siguiente registro |
| READ SEQ | 0xFAF5 | Leer siguiente registro |

### Lectura por Clave
| Opcode | Valor | Descripcion |
|--------|-------|-------------|
| READ RANDOM | 0xFAF6 | Leer por clave exacta |
| START EQ | 0xFAE9 | Posicionar en clave igual |
| START GE | 0xFAEB | Posicionar en clave >= |

### Informacion
| Opcode | Valor | Descripcion |
|--------|-------|-------------|
| GET INFO | 0x0006 | Obtener atributos del archivo |

## Flujo de Lectura

```
1. setupEnvironment()
   - Set COBCONFIG -> C:\Siigo\EXTFH.CFG
   - Set COBOPT -> C:\Siigo\COBOPT.CFG (si existe)
   - Agregar directorio de DLLs al PATH

2. findDLL()
   - Buscar cblrtsm.dll en: COBDIR, C:\Siigo\, C:\Microfocus\,
     Program Files\Micro Focus\Visual COBOL\,
     Program Files\Micro Focus\COBOL Server\

3. OpenIsamFile(path)
   - Crear FCD3 (280 bytes) + KDB
   - Configurar: organization=indexed, access=dynamic, open=input
   - Llamar EXTFH(OpOpenInput, &fcd)
   - Si status 9/065 o 9/068 -> retry hasta 3 veces

4. ReadAllRecords(fcd)
   - EXTFH(OpStepFirst) -> posicionar al inicio (con lock retry)
   - Loop: EXTFH(OpStepNext) hasta status != 0 (con lock retry)
   - Cada iteracion: copiar record buffer a slice de Go

5. CloseIsamFile(fcd)
   - EXTFH(OpClose)
```

## Lock Retry

Cuando el archivo esta bloqueado por Siigo:

```
Status 9/065 = archivo bloqueado (file lock)
Status 9/068 = registro bloqueado (record lock)
```

El wrapper reintenta automaticamente:
- **MaxLockRetries**: 3 intentos
- **LockRetryDelay**: 200ms entre intentos
- Se aplica a **todas** las operaciones: OPEN, STEP FIRST/NEXT/PREV, READ RANDOM, START

## Environment Auto-Setup

El wrapper configura automaticamente las variables de entorno necesarias:

```
COBCONFIG = C:\Siigo\EXTFH.CFG    (configuracion del file handler)
COBOPT    = C:\Siigo\COBOPT.CFG   (opciones del runtime COBOL)
PATH     += directorio de cblrtsm.dll
```

### EXTFH.CFG
```ini
[XFH-DEFAULT]
IGNORELOCK=ON       # Permite lectura concurrente
IDXFORMAT=8         # Formato de indice Micro Focus
FILEMAXSIZE=8       # Tamano maximo archivo (GB)
INDEXCOUNT=32       # Maximo indices por archivo
SEQDATBUF=8192      # Buffer lectura secuencial
FASTREAD=ON         # Lectura rapida
KEYCHECK=OFF        # No verificar claves
READSEMA=OFF        # Sin semaforos de lectura
```

## ReadIsamFile vs ReadIsamFileWithMeta

### ReadIsamFile(path string) ([][]byte, int, error)
API principal. Retorna registros como slices de bytes + tamano de registro.

### ReadIsamFileWithMeta(path string) ([][]byte, IsamFileMeta, error)
API extendida con diagnosticos:

```go
type IsamFileMeta struct {
    RecSize         int       // Tamano del registro
    RecordCount     int       // Registros encontrados
    ExpectedRecords int       // Registros esperados (del header)
    NumKeys         int       // Numero de claves (solo EXTFH)
    Format          int       // Formato del archivo (solo EXTFH)
    Keys            []KeyInfo // Claves con offset/length de componentes (solo EXTFH)
    HasIndex        bool      // Si existe archivo .idx
    UsedEXTFH       bool      // true=EXTFH, false=binary fallback
    DLLPath         string    // Ruta de cblrtsm.dll usada
}
```

### ReadIsamFileIndexed(path, keyFn) (*IsamIndex, error)
Lee registros y construye un indice en memoria con un key extractor manual.

### ReadIsamFileAutoIndexed(path, fallbackKeyFn) (*IsamIndex, error)
Lee registros y construye un indice usando la clave primaria descubierta del KDB.
Si EXTFH no esta disponible o el KDB no tiene info de componentes, usa fallbackKeyFn.

### IsamIndex - Busquedas por clave en memoria
```go
idx, _ := isam.ReadIsamFileAutoIndexed(`C:\DEMOS01\Z17`, fallbackKeyFn)

rec := idx.Lookup("G0010000000000020")   // primer registro con esa clave
recs := idx.LookupAll("G001...")         // todos con esa clave
exists := idx.Has("G001...")             // verificar existencia
keys := idx.Keys()                       // claves unicas
idx.ForEach(func(key string, rec []byte) bool { ... })
```

## Binary Fallback

Si `cblrtsm.dll` no esta disponible, `ReadIsamFile` usa el lector binario:

1. Lee todo el archivo en memoria
2. Valida header: magic 0x33FE o 0x30xx
3. Extrae recSize (offset 0x38) y expectedRecords (offset 0x40)
4. Escanea desde offset 0x800 buscando marcadores de registro
5. Valida status nibble del marcador (0x00, 0x10, ..., 0xE0)
6. Verifica texto legible en los primeros 30 bytes
7. Log warning si registros encontrados != expectedRecords

### Validacion de Header (IsamHeader)
```go
type IsamHeader struct {
    Magic           uint16 // 0x33FE o 0x30xx
    RecordSize      int    // Offset 0x38, big-endian 16-bit
    ExpectedRecords int    // Offset 0x40, big-endian 32-bit
    HasIndex        bool   // Existe .idx
    IsValid         bool   // Magic reconocido
}
```

## Por que Fallback?

El fallback binario existe porque:
1. `cblrtsm.dll` puede no estar instalada en todas las maquinas
2. La DLL es de 64-bit pero podria haber entornos de 32-bit
3. Errores de carga de DLL no deben bloquear la lectura de datos
4. Para desarrollo/testing sin Siigo instalado

El EXTFH es preferido porque:
- Lee correctamente los indices y la estructura del archivo
- Maneja automaticamente formatos especiales (como Z06 con recSize=4096)
- Respeta la configuracion de EXTFH.CFG
- Es la misma API que usa Siigo internamente
