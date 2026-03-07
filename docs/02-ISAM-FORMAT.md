# Formato de Archivos ISAM de Siigo (Micro Focus IDXFORMAT=8)

## ¿Qué es ISAM?

ISAM (Indexed Sequential Access Method) es un formato de archivos de datos con índices, usado ampliamente en COBOL. En Siigo, cada "tabla" se almacena como 2 archivos:

- **Archivo de datos** (sin extensión): ej. `Z17` — contiene los registros reales
- **Archivo de índices** (`.idx`): ej. `Z17.idx` — contiene los índices para búsqueda rápida

Siigo usa **IDXFORMAT=8**, que es un formato propietario de Micro Focus.

## Configuración del File Handler

Archivo: `C:\Siigo\EXTFH.CFG`
```ini
[XFH-DEFAULT]
IGNORELOCK=ON      # Ignorar bloqueos (permite lectura concurrente)
IDXFORMAT=8        # Formato de índice Micro Focus propietario
FILEMAXSIZE=8      # Tamaño máximo de archivo en GB
INDEXCOUNT=32       # Número máximo de índices por archivo
SEQDATBUF=8192     # Buffer de lectura secuencial
FASTREAD=ON        # Lectura rápida habilitada
KEYCHECK=OFF       # No verificar claves (más rápido)
READSEMA=OFF       # No usar semáforos de lectura
```

## Estructura Binaria del Archivo de Datos

### Layout General

```
Offset      Tamaño    Contenido
──────────────────────────────────────────────
0x000       1024      Header del archivo
0x400       1024      Primera página de índice
0x800+      variable  Páginas de datos con registros
```

### Header (0x000 - 0x3FF)

Los primeros 1024 bytes contienen metadatos del archivo:

```
Offset  Bytes  Descripcion
----------------------------------------------
0x00    2      Magic signature (big-endian 16-bit):
               - 0x33FE = archivo indexado estandar (la mayoria de archivos Z)
               - 0x30xx = variante (ej: Z06 usa 0x3000)
0x02    6      Reservado
0x08    16     Timestamp de creacion (texto ASCII: "YYYYMMDDHHMMSSCC")
0x18    16     Timestamp de modificacion (texto ASCII)
0x28    2      Marcador de version
0x2A    2      Flags
0x38    2      **TAMANO DEL REGISTRO** (big-endian 16-bit) -- CRITICO
0x3A    2      Tamano del registro (repetido)
0x3C    4      Reservado
0x40    4      **NUMERO DE REGISTROS** (big-endian 32-bit) -- conteo esperado
0x48    2      Flags de indice
```

**Campos criticos**:
- **Offset 0x00**: Magic signature. Si no es `0x33FE` ni `0x30xx`, no es un archivo ISAM valido.
- **Offset 0x38**: Tamano del registro, codificado como **big-endian 16-bit**.
- **Offset 0x40**: Conteo de registros esperado, codificado como **big-endian 32-bit**. Util para validar que el scanner encontro todos los registros.

### Archivos de Indice (.idx)

Cada archivo ISAM tiene un archivo de indice companero (ej: `Z17` + `Z17.idx`). El archivo .idx contiene los B-tree indexes para busqueda por clave. Si el archivo .idx no existe, el archivo puede ser secuencial (sin indices).

### Lectura del Header (Go)

```go
// Leer magic signature
magic := binary.BigEndian.Uint16(data[0x00:0x02])
// 0x33FE = indexado estandar, 0x30xx = variante

// Leer tamano de registro
recSize := int(binary.BigEndian.Uint16(data[0x38:0x3A]))
// Ejemplo: 0x05, 0x9E -> recSize = 1438

// Leer conteo de registros esperado
expectedRecords := int(binary.BigEndian.Uint32(data[0x40:0x44]))
```

### Páginas de Índice (0x400+)

Las páginas de índice tienen este formato:
```
Offset  Bytes  Descripción
──────────────────────────────────────────────
0x00    2      Marcador de página: 0x33FE
0x02    2      Información de página
0x04+   var    Entradas de índice, cada una:
               - 4 bytes: puntero al registro en el archivo de datos
               - N bytes: valor de la clave (longitud depende de la definición del índice)
```

Las claves son texto ASCII/EBCDIC de longitud fija.

### Registros de Datos

Cada registro en las paginas de datos tiene esta estructura:
```
Offset  Bytes  Descripcion
----------------------------------------------
0x00    1      Byte de flags/status:
               - Nibble alto (bits 4-7): status del registro
               - Nibble bajo (bits 0-3): byte alto del tamano del registro
0x01    1      Byte bajo del tamano del registro
0x02    N      Datos del registro (N = tamano del registro)
```

**Status nibbles validos** (nibble alto del primer byte):
```
0x00 = sin flags
0x10 = flag 1
0x20 = flag 2
0x40 = registro activo (el mas comun)
0x60 = activo + flag
0x80 = marcado/modificado
0xA0 = modificado + flag
0xC0 = estado alterno
0xE0 = estado alterno + flag
```

**Ejemplo**: Para un registro de tamano 0x059E (1438 bytes):
- `recHi = 0x05`, `recLo = 0x9E`
- Marcador activo: `0x45` `0x9E` (0x40 status + 0x05 recHi)
- Marcador modificado: `0x85` `0x9E` (0x80 status + 0x05 recHi)
- Marcador alterno: `0xE5` `0x9E` (0xE0 status + 0x05 recHi)

### Como Encontrar Registros (Algoritmo - Go)

El lector binario esta implementado en `siigo-sync/isam/reader.go` y `siigo-app/isam/reader.go`.

**Pasos principales:**

1. Validar header (magic signature 0x33FE o 0x30xx)
2. Leer recSize de offset 0x38 y expectedRecords de offset 0x40
3. Escanear desde offset 0x800 buscando marcadores de registro
4. Validar que el status nibble es valido (0x00-0xE0 en incrementos de 0x20)
5. Verificar texto legible en los primeros 30 bytes del registro
6. Comparar registros encontrados vs expectedRecords (log warning si difieren)

```go
// Marcador de registro: [statusNibble|recHi] [recLo]
recHi := byte((recSize >> 8) & 0xFF)
recLo := byte(recSize & 0xFF)

for pos := 0x800; pos < len(data)-recSize; pos++ {
    if data[pos+1] == recLo && (data[pos]&0x0F) == recHi {
        statusNibble := data[pos] & 0xF0
        if !validStatusNibbles[statusNibble] {
            continue
        }
        // Verificar texto legible, extraer registro...
    }
}
```

### API Unificada: ReadIsamFile

Ambos proyectos exponen `ReadIsamFile(path)` que:
1. Intenta abrir via EXTFH (cblrtsm.dll) si esta disponible
2. Si falla, usa el lector binario como fallback
3. Retorna `([][]byte, int, error)` — registros, recSize, error

Tambien existe `ReadIsamFileWithMeta(path)` que retorna un `IsamFileMeta` con diagnosticos adicionales (numKeys, format, hasIndex, usedEXTFH, etc).

## Encoding de los Datos

Los datos dentro de los registros usan **Windows-1252** (Windows Latin-1), que incluye caracteres espanoles (a, e, i, o, u con tilde, n con tilde).

```go
import "golang.org/x/text/encoding/charmap"

decoder := charmap.Windows1252.NewDecoder()
text, _ := decoder.String(string(recordBytes))
```

## Formato de los Campos dentro de un Registro

Los registros COBOL usan campos de **longitud fija** con padding de espacios:

```
Tipo        Formato                     Ejemplo
───────────────────────────────────────────────────────────
Texto       Padding derecho con espacios "PROVEEDORES             "
Número      Padding izquierdo con ceros  "00008300456780"
Fecha       YYYYMMDD en texto            "20140725"
Booleano    "S"/"N" o "1"/"0"           "S"
Vacío       Espacios o nulos            "        " o 0x00 bytes
Monetario   Número con decimales impl.   "00001500000" (= 15000.00)
```

Los campos numéricos COBOL típicamente usan formato PIC 9(n) o PIC 9(n)V9(m) donde V es el punto decimal implícito.

## Tamaños de Registro por Archivo (Verificados)

| Archivo | RecSize (dec) | RecSize (hex) | Descripción |
|---------|---------------|---------------|-------------|
| Z17 | 1,438 | 0x59E | Terceros |
| Z06 | 4,096 | 0x1000 | Productos |
| Z49 | 2,295 | 0x8F7 | Movimientos |
| Z03YYYY | 1,152 | 0x480 | Mov. contables |
| Z90ES | 250 | 0xFA | Módulos/permisos |
| Z91PRO | 254 | 0xFE | Catálogo programas |
| Z003 | 61 | 0x3D | Usuarios |
| Z70 | ~300 | ~0x12C | Comprobantes |
| C03 | 512 | 0x200 | Plan de cuentas |
| C05 | 81 | 0x51 | Config empresa |
| ZDANE | 256 | 0x100 | Ciudades |
| Z09YYYY | 1,152 | 0x480 | Cartera |
| Z04YYYY | 3,520 | 0xDC0 | Detalle movimientos |
| N03YYYY | 1,152 | 0x480 | Nómina mov. |
| N04YYYY | 3,520 | 0xDC0 | Nómina detalle |
| Z9001ES | 260 | 0x104 | Configuración |
| INF | 1,664 | 0x680 | Informes |
| Z082014 | 2,560 | 0xA00 | (por investigar) |

## Notas Importantes

1. **Usar EXTFH cuando sea posible** — El wrapper EXTFH (via cblrtsm.dll) lee los archivos correctamente usando el runtime de Micro Focus. El lector binario es un fallback.
2. **No escribir en los archivos ISAM** — La estructura es compleja (indices, nodos B-tree, etc.). Escribir mal corromperia los datos.
3. **Los archivos sin `.idx`** correspondiente pueden ser archivos secuenciales simples, no indexados.
4. **Archivos con sufijo de ano** (Z032016, Z492014) contienen datos de ese ano fiscal.
5. **El archivo Z06 tiene recSize=4096** que coincide con el tamano de pagina, lo que sugiere un formato especial. El backup Z0620171114 tiene recSize=2286 que es el tamano real del registro de productos.
6. **Lock retry** — EXTFH reintenta automaticamente 3 veces con 200ms de delay cuando el archivo esta bloqueado (status 9/065 o 9/068).
7. **Environment auto-setup** — El wrapper configura automaticamente COBCONFIG, COBOPT, y PATH para el runtime de Micro Focus.
