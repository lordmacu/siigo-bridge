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
Offset  Bytes  Descripción
──────────────────────────────────────────────
0x00    2      Marcador de archivo: 0x33FE o 0x30xx
0x02    6      Reservado
0x08    16     Timestamp de creación (texto ASCII: "YYYYMMDDHHMMSSCC")
0x18    16     Timestamp de modificación (texto ASCII)
0x28    2      Marcador de versión
0x2A    2      Flags
0x38    2      **TAMAÑO DEL REGISTRO** (big-endian 16-bit) ← CRÍTICO
0x3A    2      Tamaño del registro (repetido)
0x3C    4      Reservado
0x40    4      Número de registros (estimado)
0x48    2      Flags de índice
```

**Lo más importante**: El tamaño del registro está en **offset 0x38**, codificado como **big-endian 16-bit**.

### Lectura del Tamaño de Registro (C#)

```csharp
byte[] header = new byte[128];
using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
{
    fs.Read(header, 0, 128);
}
int recordSize = (header[0x38] << 8) | header[0x39];
// Ejemplo: header[0x38]=0x05, header[0x39]=0x9E → recordSize = 1438
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

Cada registro en las páginas de datos tiene esta estructura:
```
Offset  Bytes  Descripción
──────────────────────────────────────────────
0x00    1      Byte de flags/status:
               - Nibble alto (bits 4-7): flags (4=activo, 8=eliminado?)
               - Nibble bajo (bits 0-3): byte alto del tamaño del registro
0x01    1      Byte bajo del tamaño del registro
0x02    N      Datos del registro (N = tamaño del registro)
```

**Ejemplo**: Para un registro de tamaño 0x059E (1438 bytes):
- `recHi = 0x05`, `recLo = 0x9E`
- El marcador del registro es: `0x45` `0x9E` (donde 0x45 = 0x40 flags + 0x05 recHi)
- O puede ser: `0xE5` `0x9E` (con flags diferentes)

### Cómo Encontrar Registros (Algoritmo)

```csharp
static List<byte[]> ReadIsamRecords(string filePath)
{
    var records = new List<byte[]>();

    byte[] data;
    using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
    {
        data = new byte[fs.Length];
        fs.Read(data, 0, data.Length);
    }

    // 1. Leer tamaño de registro del header
    int recSize = (data[0x38] << 8) | data[0x39];
    if (recSize <= 0 || recSize > 60000) return records;

    // 2. Calcular bytes del marcador
    byte recHi = (byte)((recSize >> 8) & 0xFF);  // Byte alto del tamaño
    byte recLo = (byte)(recSize & 0xFF);          // Byte bajo del tamaño

    // 3. Escanear desde offset 0x800 (después del header + primera página de índice)
    for (int pos = 0x800; pos < data.Length - recSize; pos++)
    {
        // Buscar marcador: segundo byte = recLo, nibble bajo del primero = recHi
        if (data[pos + 1] == recLo && (data[pos] & 0x0F) == recHi)
        {
            // Verificar que contiene texto legible (no es basura)
            int textStart = pos + 2;
            int readableCount = 0;
            for (int i = textStart; i < textStart + 30 && i < data.Length; i++)
            {
                if (data[i] >= 0x20 && data[i] < 0xFF) readableCount++;
            }

            if (readableCount > 15) // Al menos la mitad legible
            {
                byte[] record = new byte[recSize];
                Array.Copy(data, textStart, record, 0, Math.Min(recSize, data.Length - textStart));
                records.Add(record);
                pos += recSize; // Saltar al siguiente registro potencial
            }
        }
    }

    return records;
}
```

## Encoding de los Datos

Los datos dentro de los registros usan **Windows-1252** (Windows Latin-1), que incluye caracteres españoles:
- á, é, í, ó, ú
- ñ, Ñ
- ¿, ¡

```csharp
var encoding = Encoding.GetEncoding(1252);
string text = encoding.GetString(recordBytes);
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

1. **Siempre abrir con `FileShare.ReadWrite`** — Siigo puede tener los archivos abiertos y bloqueados.
2. **No escribir en los archivos ISAM** — La estructura es compleja (índices, nodos B-tree, etc.). Escribir mal corrompería los datos.
3. **Los archivos sin `.idx`** correspondiente pueden ser archivos secuenciales simples, no indexados.
4. **Archivos con sufijo de año** (Z032016, Z492014) contienen datos de ese año fiscal.
5. **El archivo Z06 tiene recSize=4096** que coincide con el tamaño de página, lo que sugiere un formato especial. El backup Z0620171114 tiene recSize=2286 que es el tamaño real del registro de productos.
